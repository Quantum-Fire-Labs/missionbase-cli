package main

// Resolving questions on the owner's behalf: task answer records answers the
// owner gave somewhere else (in chat, say) and task dismiss closes questions
// that no longer need an answer. Both send PATCH /api/v1/tasks/:id/questions
// as the acting agent; the task page shows "Answered in chat · recorded by
// <agent>" and "Dismissed by <agent> · <reason>". See README "Agent questions".

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultAnswerSource = "in chat"
	maxAnswerSource     = 100
)

const taskAnswerUsage = `usage: missionbase-agent task answer <task-id> <question-id> "the owner's answer" [--option N] [--source TEXT] [--json]
       missionbase-agent task answer <task-id> --file PATH [--source TEXT] [--json]
  Records an answer the owner gave somewhere else, in their words. --source says where (default "in chat");
  the task page shows "Answered in chat · recorded by <agent>".
  --option N picks the question's Nth option as the answer (text, if given too, follows it).
  --file is a JSON array: [{"id": 12, "answer": "text", "option": 1}] (answer and/or option).`

const taskDismissUsage = `usage: missionbase-agent task dismiss <task-id> <question-id>... [--reason TEXT] [--json]
  Dismisses questions that no longer need an answer. The reason shows on the task page.`

// recordedAnswer is one answer to record, from the arguments or a --file entry.
type recordedAnswer struct {
	ID     int64  `json:"id"`
	Answer string `json:"answer"`
	Option int    `json:"option"`
}

type resolveResponse struct {
	Task          workTask       `json:"task"`
	Questions     []taskQuestion `json:"questions"`
	Answered      []int64        `json:"answered"`
	Dismissed     []int64        `json:"dismissed"`
	Skipped       []int64        `json:"skipped"`
	OpenRemaining int            `json:"open_remaining"`
}

func taskAnswer(args []string) error {
	var positional []string
	file, source, jsonOut := "", defaultAnswerSource, false
	option, optionSet, sourceSet := 0, false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--option", "--source", "--file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value\n%s", args[i], taskAnswerUsage)
			}
			flag, value := args[i], args[i+1]
			i++
			switch flag {
			case "--option":
				if optionSet {
					return fmt.Errorf("--option can be given only once")
				}
				n, err := strconv.Atoi(strings.TrimSpace(value))
				if err != nil || n < 1 || n > maxQuestionOptions {
					return fmt.Errorf("--option must be an option number from 1 to %d, got %q", maxQuestionOptions, value)
				}
				option, optionSet = n, true
			case "--source":
				if sourceSet {
					return fmt.Errorf("--source can be given only once")
				}
				source, sourceSet = strings.TrimSpace(value), true
			case "--file":
				if file != "" {
					return fmt.Errorf("--file can be given only once")
				}
				if strings.TrimSpace(value) == "" {
					return fmt.Errorf("--file requires a file path")
				}
				file = value
			}
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(taskAnswerUsage)
			return nil
		default:
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				return fmt.Errorf("unknown task answer option %q\n%s", args[i], taskAnswerUsage)
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) == 0 {
		return fmt.Errorf("%s", taskAnswerUsage)
	}
	path, err := taskPath(positional[0], "/questions")
	if err != nil {
		return err
	}
	if source == "" {
		return fmt.Errorf("--source can't be blank; say where the owner answered, e.g. \"in chat\"")
	}
	if len(source) > maxAnswerSource {
		return fmt.Errorf("--source must be at most %d characters", maxAnswerSource)
	}

	var answers []recordedAnswer
	if file != "" {
		if len(positional) > 1 || optionSet {
			return fmt.Errorf("use either <question-id> \"answer\" [--option N], or --file, not both")
		}
		if answers, err = readAnswersFile(file); err != nil {
			return err
		}
	} else {
		if len(positional) < 2 {
			return fmt.Errorf("a question id is required (see them with: missionbase-agent task questions %s --status open)\n%s", positional[0], taskAnswerUsage)
		}
		if len(positional) > 3 {
			return fmt.Errorf("unexpected argument %q (quote the answer, or use --file for several)\n%s", positional[3], taskAnswerUsage)
		}
		id, err := questionID(positional[1])
		if err != nil {
			return err
		}
		answer := recordedAnswer{ID: id, Option: option}
		if len(positional) == 3 {
			answer.Answer = positional[2]
		}
		answers = []recordedAnswer{answer}
	}

	seen := map[int64]bool{}
	needsOptions := false
	for i := range answers {
		a := &answers[i]
		a.Answer = strings.TrimSpace(normalizeAgentAuthoredBody(a.Answer))
		if a.Answer == "" && a.Option == 0 {
			return fmt.Errorf("question %d: give the owner's answer as text, --option N, or both", a.ID)
		}
		if seen[a.ID] {
			return fmt.Errorf("question %d is answered more than once", a.ID)
		}
		seen[a.ID] = true
		needsOptions = needsOptions || a.Option > 0
	}

	// Options are picked by number, so read the questions to turn numbers into text.
	if needsOptions {
		_, listed, err := fetchQuestions(path)
		if err != nil {
			return err
		}
		byID := map[int64]taskQuestion{}
		for _, q := range listed.Questions {
			byID[q.ID] = q
		}
		for i := range answers {
			a := &answers[i]
			if a.Option == 0 {
				continue
			}
			q, ok := byID[a.ID]
			if !ok {
				return fmt.Errorf("question %d is not on task #%s", a.ID, positional[0])
			}
			if a.Option > len(q.Options) {
				return fmt.Errorf("question %d has %s; --option %d doesn't exist", a.ID, plural(len(q.Options), "option"), a.Option)
			}
			chosen := q.Options[a.Option-1]
			if a.Answer != "" {
				chosen += " — " + a.Answer
			}
			a.Answer = chosen
		}
	}

	payload := map[string]any{"answers": answerMap(answers), "source": source}
	body, err := agentDo(http.MethodPatch, path, payload)
	if err != nil {
		return err
	}
	if jsonOut {
		fmt.Println(string(body))
		return nil
	}
	var result resolveResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("unexpected task answer response: %w", err)
	}
	fmt.Printf("Recorded %s given %s on #%d %q\n", plural(len(result.Answered), "answer"), source, result.Task.ID, result.Task.Title)
	printResolved(result, result.Answered)
	return nil
}

func taskDismiss(args []string) error {
	var positional []string
	reason, reasonSet, jsonOut := "", false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--reason":
			if i+1 >= len(args) {
				return fmt.Errorf("--reason requires a value\n%s", taskDismissUsage)
			}
			if reasonSet {
				return fmt.Errorf("--reason can be given only once")
			}
			reason, reasonSet = strings.TrimSpace(normalizeAgentAuthoredBody(args[i+1])), true
			i++
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(taskDismissUsage)
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown task dismiss option %q\n%s", args[i], taskDismissUsage)
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) == 0 {
		return fmt.Errorf("%s", taskDismissUsage)
	}
	path, err := taskPath(positional[0], "/questions")
	if err != nil {
		return err
	}
	if len(positional) < 2 {
		return fmt.Errorf("at least one question id is required (to dismiss all open questions, use: missionbase-agent task attention clear %s)\n%s", positional[0], taskDismissUsage)
	}
	if reasonSet && reason == "" {
		return fmt.Errorf("--reason can't be blank")
	}
	var ids []int64
	seen := map[int64]bool{}
	for _, arg := range positional[1:] {
		id, err := questionID(arg)
		if err != nil {
			return err
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}

	payload := map[string]any{"dismiss": ids}
	if reason != "" {
		payload["reason"] = reason
	}
	body, err := agentDo(http.MethodPatch, path, payload)
	if err != nil {
		return err
	}
	if jsonOut {
		fmt.Println(string(body))
		return nil
	}
	var result resolveResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("unexpected task dismiss response: %w", err)
	}
	fmt.Printf("Dismissed %s on #%d %q\n", plural(len(result.Dismissed), "question"), result.Task.ID, result.Task.Title)
	printResolved(result, result.Dismissed)
	return nil
}

func questionID(value string) (int64, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("question id must be a number, got %q", value)
	}
	return id, nil
}

// answerMap is the answers param: {"<question-id>": "text"}.
func answerMap(answers []recordedAnswer) map[string]string {
	out := make(map[string]string, len(answers))
	for _, a := range answers {
		out[strconv.FormatInt(a.ID, 10)] = a.Answer
	}
	return out
}

// printResolved prints the questions just resolved, the ones skipped because
// they were no longer open, and how many are still open.
func printResolved(result resolveResponse, resolved []int64) {
	done := map[int64]bool{}
	for _, id := range resolved {
		done[id] = true
	}
	for _, q := range result.Questions {
		if done[q.ID] {
			printQuestion(q, "  ", false)
		}
	}
	if len(result.Skipped) > 0 {
		skipped := append([]int64(nil), result.Skipped...)
		sort.Slice(skipped, func(i, j int) bool { return skipped[i] < skipped[j] })
		ids := make([]string, len(skipped))
		for i, id := range skipped {
			ids[i] = strconv.FormatInt(id, 10)
		}
		fmt.Printf("Skipped (already answered or dismissed): %s\n", strings.Join(ids, ", "))
	}
	if result.OpenRemaining == 0 {
		fmt.Println("No questions left open")
	} else {
		fmt.Printf("%s still open\n", plural(result.OpenRemaining, "question"))
	}
}

// readAnswersFile reads task answer --file: a JSON array of {"id", "answer", "option"}.
func readAnswersFile(path string) ([]recordedAnswer, error) {
	if path == "-" {
		return nil, fmt.Errorf("stdin is not supported for --file; write the answers to a file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read answers file %q: %w", path, err)
	}
	const shape = `a JSON array of {"id": 12, "answer": "text", "option": 1}`
	if len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '[' {
		return nil, fmt.Errorf("%s must be %s", path, shape)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var answers []recordedAnswer
	if err := decoder.Decode(&answers); err != nil {
		return nil, fmt.Errorf("%s is not a valid answers file (%s): %w", path, shape, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s has extra content after the JSON array", path)
	}
	if len(answers) == 0 {
		return nil, fmt.Errorf("%s has no answers", path)
	}
	for i, a := range answers {
		if a.ID <= 0 {
			return nil, fmt.Errorf("%s: answer %d needs a question id", path, i+1)
		}
		if a.Option < 0 || a.Option > maxQuestionOptions {
			return nil, fmt.Errorf("%s: answer %d: option must be from 1 to %d", path, i+1, maxQuestionOptions)
		}
	}
	return answers, nil
}
