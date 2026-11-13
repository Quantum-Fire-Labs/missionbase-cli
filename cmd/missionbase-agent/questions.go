package main

// Agent questions: ask the task's owner 1–5 questions (task ask) and read or
// wait for the answers (task questions). See README "Agent questions".

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/httpclient"
)

const (
	maxQuestionsPerAsk = 5
	maxQuestionOptions = 4
)

var validQuestionStatuses = []string{"open", "answered", "dismissed"}

// questionsPollInterval is how often task questions --wait polls. Tests shorten it.
var questionsPollInterval = 20 * time.Second

type questionPerson struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Handle string `json:"handle"`
}

func (p *questionPerson) display() string {
	if p == nil {
		return ""
	}
	for _, value := range []string{p.Name, p.Handle, p.Slug} {
		if value != "" {
			return value
		}
	}
	return p.Type
}

type taskQuestion struct {
	ID          int64           `json:"id"`
	Batch       string          `json:"batch"`
	Position    int             `json:"position"`
	Body        string          `json:"body"`
	Context     *string         `json:"context"`
	Options     []string        `json:"options"`
	Status      string          `json:"status"`
	AskedBy     *questionPerson `json:"asked_by"`
	AskedAt     string          `json:"asked_at"`
	AnswerBody  *string         `json:"answer_body"`
	AnsweredBy  *questionPerson `json:"answered_by"`
	DismissedBy *questionPerson `json:"dismissed_by"`
}

// questionInput is one question as sent to POST /api/v1/tasks/:id/questions
// and as read from task ask --file.
type questionInput struct {
	Body    string   `json:"body"`
	Context string   `json:"context,omitempty"`
	Options []string `json:"options,omitempty"`
}

func taskPageURL(taskID int64) string {
	cfg, err := config.LoadAgent()
	if err != nil || cfg.BaseURL == "" {
		return ""
	}
	return strings.TrimRight(cfg.BaseURL, "/") + "/tasks/" + strconv.FormatInt(taskID, 10)
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// indentBlock indents every line after the first so multi-line Markdown
// stays under its label.
func indentBlock(text, indent string) string {
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	return strings.ReplaceAll(text, "\n", "\n"+indent)
}

// printQuestion prints one question as "N. [status] body" plus its context,
// options and the answer or dismissal.
func printQuestion(q taskQuestion, indent string, showStatus bool) {
	number := strconv.Itoa(q.Position) + ". "
	if q.Position == 0 {
		number = "- "
	}
	head := indent + number
	if showStatus {
		head += "[" + q.Status + "] "
	}
	inner := indent + strings.Repeat(" ", len(number))
	fmt.Println(head + indentBlock(q.Body, inner))
	if q.Context != nil && strings.TrimSpace(*q.Context) != "" {
		fmt.Println(inner + "Context: " + indentBlock(*q.Context, inner+"  "))
	}
	if len(q.Options) > 0 {
		fmt.Println(inner + "Options: " + strings.Join(q.Options, " · "))
	}
	switch q.Status {
	case "answered":
		label := "Answer"
		if who := q.AnsweredBy.display(); who != "" {
			label += " (" + who + ")"
		}
		answer := ""
		if q.AnswerBody != nil {
			answer = *q.AnswerBody
		}
		fmt.Println(inner + label + ": " + indentBlock(answer, inner+"  "))
	case "dismissed":
		line := inner + "Dismissed"
		if who := q.DismissedBy.display(); who != "" {
			line += " by " + who
		}
		fmt.Println(line)
	}
}

// printQuestionList prints questions grouped by batch, each batch headed by
// who asked it and when.
func printQuestionList(questions []taskQuestion) {
	batch := "\x00"
	for _, q := range questions {
		if q.Batch != batch {
			batch = q.Batch
			head := "Asked"
			if who := q.AskedBy.display(); who != "" {
				head += " by " + who
			}
			if q.AskedAt != "" {
				head += " at " + q.AskedAt
			}
			fmt.Println(head)
		}
		printQuestion(q, "  ", true)
	}
}

// ---- task ask -------------------------------------------------------------------

const taskAskUsage = `usage: missionbase-agent task ask <task-id> "question" [--context TEXT] [--option TEXT]... [--json]
       missionbase-agent task ask <task-id> --file PATH [--json]
  --option is repeatable (at most 4). --file is a JSON array of 1 to 5 questions:
  [{"body": "question", "context": "optional", "options": ["optional", "up to 4"]}]`

func taskAsk(args []string) error {
	taskID, file, jsonOut := "", "", false
	var body, context *string
	var options []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--context", "--option", "--file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value\n%s", args[i], taskAskUsage)
			}
			value := args[i+1]
			i++
			switch args[i-1] {
			case "--context":
				if context != nil {
					return fmt.Errorf("--context can be given only once")
				}
				context = &value
			case "--option":
				options = append(options, value)
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
			fmt.Println(taskAskUsage)
			return nil
		default:
			switch {
			case strings.HasPrefix(args[i], "-") && args[i] != "-":
				return fmt.Errorf("unknown task ask option %q\n%s", args[i], taskAskUsage)
			case taskID == "":
				taskID = args[i]
			case body == nil:
				value := args[i]
				body = &value
			default:
				return fmt.Errorf("unexpected argument %q (quote the question, or use --file for several)\n%s", args[i], taskAskUsage)
			}
		}
	}
	if taskID == "" {
		return fmt.Errorf("%s", taskAskUsage)
	}
	path, err := taskPath(taskID, "/questions")
	if err != nil {
		return err
	}

	var questions []questionInput
	if file != "" {
		if body != nil || context != nil || len(options) > 0 {
			return fmt.Errorf("use either a question with --context/--option, or --file, not both")
		}
		if questions, err = readQuestionsFile(file); err != nil {
			return err
		}
	} else {
		if body == nil {
			return fmt.Errorf("a question is required: task ask <task-id> \"question\", or --file PATH for several\n%s", taskAskUsage)
		}
		q := questionInput{Body: *body, Options: options}
		if context != nil {
			q.Context = *context
		}
		questions = []questionInput{q}
	}
	if err := normalizeQuestions(questions, file != ""); err != nil {
		return err
	}

	response, err := agentDo(http.MethodPost, path, map[string]any{"questions": questions})
	if err != nil {
		return err
	}
	if jsonOut {
		fmt.Println(string(response))
		return nil
	}
	var result struct {
		Questions []taskQuestion `json:"questions"`
		Task      workTask       `json:"task"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("unexpected task ask response: %w", err)
	}
	fmt.Printf("Asked %s on #%d %q\n", plural(len(result.Questions), "question"), result.Task.ID, result.Task.Title)
	for _, q := range result.Questions {
		printQuestion(q, "  ", false)
	}
	fmt.Printf("Status: %s\n", result.Task.label())
	if link := taskPageURL(result.Task.ID); link != "" {
		fmt.Println(link)
	}
	fmt.Printf("Wait for answers with: missionbase-agent task questions %d --wait\n", result.Task.ID)
	return nil
}

// readQuestionsFile reads task ask --file: a JSON array of question objects.
// Unknown keys are rejected so a typo like "question" doesn't send an empty body.
func readQuestionsFile(path string) ([]questionInput, error) {
	if path == "-" {
		return nil, fmt.Errorf("stdin is not supported for --file; write the questions to a file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read questions file %q: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '[' {
		return nil, fmt.Errorf(`%s must be a JSON array of questions: [{"body": "...", "context": "...", "options": ["..."]}]`, path)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var questions []questionInput
	if err := decoder.Decode(&questions); err != nil {
		return nil, fmt.Errorf(`%s is not a valid questions file (a JSON array of {"body", "context", "options"}): %w`, path, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s has extra content after the JSON array", path)
	}
	return questions, nil
}

// normalizeQuestions trims bodies and checks the limits the server enforces,
// so mistakes are caught before anything is sent.
func normalizeQuestions(questions []questionInput, fromFile bool) error {
	if len(questions) == 0 || len(questions) > maxQuestionsPerAsk {
		return fmt.Errorf("ask 1 to %d questions at a time (got %d)", maxQuestionsPerAsk, len(questions))
	}
	var problems []string
	for i := range questions {
		q := &questions[i]
		where := "the question"
		if fromFile {
			where = fmt.Sprintf("question %d", i+1)
		}
		q.Body = strings.TrimSpace(normalizeAgentAuthoredBody(q.Body))
		q.Context = strings.TrimSpace(normalizeAgentAuthoredBody(q.Context))
		if q.Body == "" {
			problems = append(problems, where+": body can't be blank")
		}
		if len(q.Options) > maxQuestionOptions {
			problems = append(problems, fmt.Sprintf("%s: at most %d options (got %d)", where, maxQuestionOptions, len(q.Options)))
		}
		for j, option := range q.Options {
			q.Options[j] = strings.TrimSpace(option)
			if q.Options[j] == "" {
				problems = append(problems, fmt.Sprintf("%s: option %d is blank", where, j+1))
			}
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

// ---- task questions -----------------------------------------------------------------

const taskQuestionsUsage = `usage: missionbase-agent task questions <task-id> [--status open|answered|dismissed[,...]] [--json]
       missionbase-agent task questions <task-id> --wait [--timeout DURATION] [--json]
  --wait polls every 20s until the task has no open questions, then prints the questions it waited on.
  --timeout takes a Go duration (90s, 10m, 1h) or seconds; it exits 6 if questions are still open.`

type questionsResponse struct {
	TaskID    int64          `json:"task_id"`
	OpenCount int            `json:"open_count"`
	Questions []taskQuestion `json:"questions"`
}

func taskQuestions(args []string) error {
	taskID, status, jsonOut, wait := "", "", false, false
	var timeout time.Duration
	timeoutSet := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--status":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return fmt.Errorf("--status requires a value: open, answered, dismissed (comma-separated)")
			}
			status = strings.ReplaceAll(strings.TrimSpace(args[i+1]), " ", "")
			i++
		case "--open":
			status = "open"
		case "--wait":
			wait = true
		case "--timeout":
			if i+1 >= len(args) {
				return fmt.Errorf("--timeout requires a duration such as 10m or 90s")
			}
			d, err := parseWaitTimeout(args[i+1])
			if err != nil {
				return err
			}
			timeout, timeoutSet = d, true
			i++
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(taskQuestionsUsage)
			return nil
		default:
			if strings.HasPrefix(args[i], "-") || taskID != "" {
				return fmt.Errorf("unknown task questions option %q\n%s", args[i], taskQuestionsUsage)
			}
			taskID = args[i]
		}
	}
	if taskID == "" {
		return fmt.Errorf("%s", taskQuestionsUsage)
	}
	path, err := taskPath(taskID, "/questions")
	if err != nil {
		return err
	}
	if status != "" {
		for _, s := range strings.Split(status, ",") {
			if !contains(validQuestionStatuses, s) {
				return fmt.Errorf("--status must be open, answered or dismissed (comma-separated), got %q", status)
			}
		}
	}
	if timeoutSet && !wait {
		return fmt.Errorf("--timeout is only used with --wait")
	}
	if wait {
		if status != "" {
			return fmt.Errorf("--status can't be used with --wait")
		}
		return waitForQuestions(taskID, path, timeout, jsonOut)
	}

	if status != "" {
		path = withQuery(path, url.Values{"status": {status}})
	}
	body, response, err := fetchQuestions(path)
	if err != nil {
		return err
	}
	if jsonOut {
		fmt.Println(string(body))
		return nil
	}
	printQuestionsSummary(taskID, response, response.Questions)
	return nil
}

func fetchQuestions(path string) ([]byte, questionsResponse, error) {
	var response questionsResponse
	body, err := agentDo(http.MethodGet, path, nil)
	if err != nil {
		return nil, response, err
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, response, fmt.Errorf("unexpected task questions response: %w", err)
	}
	return body, response, nil
}

func printQuestionsSummary(taskID string, response questionsResponse, shown []taskQuestion) {
	if len(shown) == 0 {
		fmt.Printf("No questions on #%s\n", taskID)
		return
	}
	fmt.Printf("#%s: %s, %d open\n", taskID, plural(len(shown), "question"), countOpen(shown))
	printQuestionList(shown)
}

func countOpen(questions []taskQuestion) int {
	n := 0
	for _, q := range questions {
		if q.Status == "open" {
			n++
		}
	}
	return n
}

func parseWaitTimeout(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("--timeout must be a positive duration such as 90s, 10m or 1h, got %q", value)
	}
	return d, nil
}

// waitForQuestions polls GET …/questions until open_count is 0. It then prints
// the questions that were open when it started (or, if none were, the latest
// batch, which the owner may have answered before the first poll).
// Network errors and 5xx responses are retried; other API errors stop it.
func waitForQuestions(taskID, path string, timeout time.Duration, jsonOut bool) error {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	var waitedOn map[int64]bool
	for {
		body, response, err := fetchQuestions(path)
		if err != nil {
			if !retryableWaitError(err) {
				return err
			}
			fmt.Fprintf(os.Stderr, "missionbase-agent: %v (retrying)\n", err)
		} else {
			if waitedOn == nil {
				waitedOn = questionsToWaitOn(response.Questions)
			}
			if response.OpenCount == 0 && countOpen(response.Questions) == 0 {
				if jsonOut {
					fmt.Println(string(body))
					return nil
				}
				var shown []taskQuestion
				for _, q := range response.Questions {
					if waitedOn[q.ID] {
						shown = append(shown, q)
					}
				}
				printQuestionsSummary(taskID, response, shown)
				return nil
			}
			if !deadline.IsZero() && !time.Now().Before(deadline) {
				if jsonOut {
					fmt.Println(string(body))
				}
				return withExitCode(exitWaitTimeout, fmt.Errorf("timed out after %s: %s still open on #%s", timeout, plural(response.OpenCount, "question"), taskID))
			}
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return withExitCode(exitWaitTimeout, fmt.Errorf("timed out after %s waiting for answers on #%s", timeout, taskID))
		}
		sleep := questionsPollInterval
		if !deadline.IsZero() {
			if remaining := time.Until(deadline); remaining < sleep {
				sleep = remaining
			}
		}
		time.Sleep(sleep)
	}
}

func questionsToWaitOn(questions []taskQuestion) map[int64]bool {
	ids := map[int64]bool{}
	for _, q := range questions {
		if q.Status == "open" {
			ids[q.ID] = true
		}
	}
	if len(ids) == 0 && len(questions) > 0 {
		latest := questions[len(questions)-1].Batch // the list is ordered by asked time
		for _, q := range questions {
			if q.Batch == latest {
				ids[q.ID] = true
			}
		}
	}
	return ids
}

func retryableWaitError(err error) bool {
	var httpErr *httpclient.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500 || httpErr.StatusCode == http.StatusTooManyRequests
	}
	var netErr *url.Error // could not reach the server
	return errors.As(err, &netErr)
}
