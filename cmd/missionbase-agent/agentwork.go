package main

// Agent work commands: start/stop work on a task, Needs Attention, project-scoped
// task listings, the changes feed and the next task. See README "Agent work".

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/httpclient"
)

// Exit codes. Everything not listed here exits 1.
const (
	exitConflict          = 3 // HTTP 409, e.g. task start while work runs elsewhere
	exitHCUnavailable     = 4 // hc is missing, failed, or could not resolve the machine/terminal
	exitProjectUnresolved = 5 // no --project, .missionbase/project.json or default project
)

// exitCodeError carries a specific process exit code to main.
type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string { return e.err.Error() }
func (e *exitCodeError) Unwrap() error { return e.err }

func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &exitCodeError{code: code, err: err}
}

// exitCodeFor maps an error returned by run to a process exit code.
func exitCodeFor(err error) int {
	var coded *exitCodeError
	if errors.As(err, &coded) {
		return coded.code
	}
	var httpErr *httpclient.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict {
		return exitConflict
	}
	return 1
}

var validStopReasons = []string{"done", "manual", "reopened", "reassigned", "terminal_gone", "agent_exited"}

// ---- hc ----------------------------------------------------------------------

const hcTimeout = 3 * time.Second

// runHC runs `hc <args>` and returns stdout. Tests replace it.
var runHC = func(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), hcTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "hc", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("hc is not installed or not on PATH")
		}
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("hc %s timed out after %s; is the hc server running?", strings.Join(args, " "), hcTimeout)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("hc %s failed: %s", strings.Join(args, " "), msg)
		}
		return nil, fmt.Errorf("hc %s failed: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

type hcMachine struct {
	ID   string // hc core id
	Name string
}

func hcLocalCore() (hcMachine, error) {
	out, err := runHC("server", "status", "--json")
	if err != nil {
		return hcMachine{}, err
	}
	var status struct {
		Core struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"core"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return hcMachine{}, fmt.Errorf("could not read `hc server status --json`: %w", err)
	}
	if status.Core.ID == "" {
		return hcMachine{}, fmt.Errorf("`hc server status --json` did not include core.id")
	}
	return hcMachine{ID: status.Core.ID, Name: status.Core.Name}, nil
}

// hcFindMachine resolves a machine as named on this core (`hc machine ls`) to
// its core id. It matches name, host, target, then id.
func hcFindMachine(name string) (hcMachine, error) {
	out, err := runHC("machine", "ls", "--json")
	if err != nil {
		return hcMachine{}, err
	}
	var listing struct {
		Machines []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Host   string `json:"host"`
			Target string `json:"target"`
		} `json:"machines"`
	}
	if err := json.Unmarshal(out, &listing); err != nil {
		return hcMachine{}, fmt.Errorf("could not read `hc machine ls --json`: %w", err)
	}
	for _, field := range []string{"name", "host", "target", "id"} {
		for _, m := range listing.Machines {
			value := map[string]string{"name": m.Name, "host": m.Host, "target": m.Target, "id": m.ID}[field]
			if value != "" && value == name && m.ID != "" {
				display := m.Name
				if display == "" {
					display = name
				}
				return hcMachine{ID: m.ID, Name: display}, nil
			}
		}
	}
	known := make([]string, 0, len(listing.Machines))
	for _, m := range listing.Machines {
		known = append(known, m.Name)
	}
	if len(known) == 0 {
		return hcMachine{}, fmt.Errorf("machine %q is not known to hc (`hc machine ls` lists none)", name)
	}
	return hcMachine{}, fmt.Errorf("machine %q is not known to hc; known machines: %s", name, strings.Join(known, ", "))
}

// resolveWorkTerminal turns --terminal ([machine/]tN, or empty for
// $HC_TERMINAL_ID) into the core id, machine name and terminal to send.
func resolveWorkTerminal(spec string) (hcMachine, string, error) {
	machineName, terminal := "", strings.TrimSpace(spec)
	if terminal == "" {
		terminal = strings.TrimSpace(os.Getenv("HC_TERMINAL_ID"))
		if terminal == "" {
			return hcMachine{}, "", withExitCode(exitHCUnavailable, fmt.Errorf("HC_TERMINAL_ID is not set; run this inside an hc terminal or pass --terminal [machine/]tN"))
		}
	} else if i := strings.LastIndex(terminal, "/"); i >= 0 {
		machineName, terminal = strings.TrimSpace(terminal[:i]), strings.TrimSpace(terminal[i+1:])
		if machineName == "" {
			return hcMachine{}, "", fmt.Errorf("--terminal %q: machine name is empty; use [machine/]tN", spec)
		}
	}
	if terminal == "" || strings.ContainsAny(terminal, " \t/") {
		return hcMachine{}, "", fmt.Errorf("--terminal %q is not a terminal address like t7 or machine/t7", spec)
	}
	if len(terminal) > 100 {
		return hcMachine{}, "", fmt.Errorf("terminal address is longer than 100 characters")
	}

	local, err := hcLocalCore()
	if err != nil {
		return hcMachine{}, "", withExitCode(exitHCUnavailable, fmt.Errorf("task start needs hc to find the machine's core id: %w", err))
	}
	if machineName == "" || machineName == local.Name || machineName == local.ID {
		return local, terminal, nil
	}
	machine, err := hcFindMachine(machineName)
	if err != nil {
		return hcMachine{}, "", withExitCode(exitHCUnavailable, err)
	}
	return machine, terminal, nil
}

// ---- project resolution --------------------------------------------------------

type resolvedProject struct {
	BoxID   string `json:"box_id"`
	BoxName string `json:"box_name,omitempty"`
	Source  string `json:"source"` // flag | project_file | default
	Path    string `json:"path,omitempty"`
}

const projectFileName = ".missionbase/project.json"

// resolveProject applies --project, then the nearest .missionbase/project.json
// from --cwd (or the current folder), then the machine's default project.
// ok is false when nothing resolves.
func resolveProject(projectFlag, cwdFlag string) (resolvedProject, bool, error) {
	if projectFlag != "" {
		if !isPositiveInteger(projectFlag) {
			return resolvedProject{}, false, fmt.Errorf("--project must be a project (box) id, got %q", projectFlag)
		}
		return resolvedProject{BoxID: projectFlag, Source: "flag"}, true, nil
	}

	dir := cwdFlag
	if dir == "" {
		var err error
		if dir, err = os.Getwd(); err != nil {
			return resolvedProject{}, false, err
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return resolvedProject{}, false, err
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return resolvedProject{}, false, fmt.Errorf("--cwd %q is not a directory", cwdFlag)
	}
	for {
		path := filepath.Join(dir, projectFileName)
		if data, err := os.ReadFile(path); err == nil {
			project, err := parseProjectFile(path, data)
			return project, err == nil, err
		} else if !errors.Is(err, os.ErrNotExist) {
			return resolvedProject{}, false, fmt.Errorf("read %s: %w", path, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	cfg, err := config.LoadAgent()
	if err != nil {
		return resolvedProject{}, false, err
	}
	if cfg.DefaultProject != "" {
		return resolvedProject{BoxID: cfg.DefaultProject, Source: "default", Path: config.CredentialsPath("missionbase-agent")}, true, nil
	}
	return resolvedProject{}, false, nil
}

func parseProjectFile(path string, data []byte) (resolvedProject, error) {
	var file struct {
		BoxID   json.RawMessage `json:"box_id"`
		BoxName string          `json:"box_name"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return resolvedProject{}, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	boxID := strings.Trim(strings.TrimSpace(string(file.BoxID)), `"`)
	if !isPositiveInteger(boxID) {
		return resolvedProject{}, fmt.Errorf("%s has no valid box_id", path)
	}
	return resolvedProject{BoxID: boxID, BoxName: file.BoxName, Source: "project_file", Path: path}, nil
}

func isPositiveInteger(value string) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && n > 0
}

func errNoProject() error {
	return withExitCode(exitProjectUnresolved, fmt.Errorf("no project here: pass --project ID, add %s, or set a machine default with `missionbase-agent project default ID` (or use --all)", projectFileName))
}

// projectFlags parses --project and --cwd, which are mutually exclusive.
type projectFlags struct {
	project string
	cwd     string
}

func (p *projectFlags) parse(args []string, i *int) (bool, error) {
	switch args[*i] {
	case "--project", "--box":
		if *i+1 >= len(args) || strings.TrimSpace(args[*i+1]) == "" {
			return true, fmt.Errorf("%s requires a project id", args[*i])
		}
		p.project = strings.TrimSpace(args[*i+1])
		*i++
		return true, nil
	case "--cwd":
		if *i+1 >= len(args) || strings.TrimSpace(args[*i+1]) == "" {
			return true, fmt.Errorf("--cwd requires a directory")
		}
		p.cwd = args[*i+1]
		*i++
		return true, nil
	}
	return false, nil
}

func (p projectFlags) validate() error {
	if p.project != "" && p.cwd != "" {
		return fmt.Errorf("use only one of --cwd or --project")
	}
	return nil
}

// ---- payloads ----------------------------------------------------------------

type workActor struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type workRunning struct {
	Agent       workActor `json:"agent"`
	MachineID   string    `json:"machine_id"`
	MachineName string    `json:"machine_name"`
	Terminal    string    `json:"terminal"`
	Since       string    `json:"since"`
	Stale       bool      `json:"stale"`
	Reason      string    `json:"reason"`
}

type workTask struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	StatusLabel string `json:"status_label"`
	Box         *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"box"`
	Assignees  []workActor  `json:"assignees"`
	Running    *workRunning `json:"running"`
	LiveStatus *struct {
		Worker string `json:"worker"`
		State  string `json:"state"`
	} `json:"live_status"`
	Attention *struct {
		Reason *string `json:"reason"`
	} `json:"attention"`
}

func (t workTask) label() string {
	if t.StatusLabel != "" {
		return t.StatusLabel
	}
	return t.Status
}

func (r workRunning) chip() string {
	who := r.Agent.Slug
	if who == "" {
		who = r.Agent.Name
	}
	where := r.MachineName
	if where == "" {
		where = r.MachineID
	}
	chip := strings.TrimSpace(who + " · " + where + " " + r.Terminal)
	if r.Stale {
		chip += " (stale)"
	}
	return chip
}

// rawTask keeps the server's task object verbatim for --json output while
// decoding the fields the CLI needs.
type rawTask struct {
	raw  json.RawMessage
	task workTask
}

func decodeRawTasks(items []json.RawMessage) ([]rawTask, error) {
	out := make([]rawTask, 0, len(items))
	for _, item := range items {
		var t workTask
		if err := json.Unmarshal(item, &t); err != nil {
			return nil, fmt.Errorf("unexpected task payload: %w", err)
		}
		out = append(out, rawTask{raw: item, task: t})
	}
	return out, nil
}

func rawMessages(tasks []rawTask) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, t.raw)
	}
	return out
}

func printJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printTaskTable(tasks []rawTask, showBox bool) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, rt := range tasks {
		t := rt.task
		columns := []string{"#" + strconv.FormatInt(t.ID, 10), t.label()}
		if showBox {
			box := ""
			if t.Box != nil {
				box = t.Box.Name
			}
			columns = append(columns, box)
		}
		columns = append(columns, t.Title)
		var extra []string
		if t.Running != nil {
			extra = append(extra, "["+t.Running.chip()+"]")
		}
		if t.LiveStatus != nil && t.LiveStatus.State == "permission" {
			extra = append(extra, "needs permission ("+t.LiveStatus.Worker+")")
		}
		if t.Attention != nil && t.Attention.Reason != nil {
			extra = append(extra, "waiting on you: "+oneLine(*t.Attention.Reason, 80))
		}
		columns = append(columns, strings.Join(extra, " "))
		fmt.Fprintln(w, strings.TrimRight(strings.Join(columns, "\t"), "\t"))
	}
	_ = w.Flush()
}

func oneLine(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) > max {
		return string([]rune(value)[:max-1]) + "…"
	}
	return value
}

// apiErrorMessage extracts error.message from an API error, if present.
func apiErrorMessage(err error) (*httpclient.HTTPError, string) {
	var httpErr *httpclient.HTTPError
	if !errors.As(err, &httpErr) {
		return nil, ""
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(httpErr.Body, &envelope)
	return httpErr, envelope.Error.Message
}

// agentDo sends an agent-acting request (slug required) and returns the body.
func agentDo(method, path string, payload any) ([]byte, error) {
	cfg, err := config.LoadAgent()
	if err != nil {
		return nil, err
	}
	if cfg.AgentSlug == "" {
		return nil, fmt.Errorf("agent slug is not set; run `missionbase-agent use <slug>` in this directory or set MISSIONBASE_AGENT_SLUG")
	}
	var body []byte
	if payload != nil {
		if body, err = json.Marshal(payload); err != nil {
			return nil, err
		}
	}
	return httpclient.New(cfg).Do(method, path, body)
}

func taskPath(taskID, suffix string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if !isPositiveInteger(taskID) {
		return "", fmt.Errorf("task id must be a number, got %q", taskID)
	}
	return "/api/v1/tasks/" + taskID + suffix, nil
}

// ---- task start / stop -----------------------------------------------------------

const taskStartUsage = "usage: missionbase-agent task start <task-id> [--terminal [machine/]tN] [--force] [--json]"

func taskStart(args []string) error {
	taskID, terminalSpec, force, jsonOut := "", "", false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--terminal":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return fmt.Errorf("--terminal requires a value like t7 or machine/t7")
			}
			terminalSpec = args[i+1]
			i++
		case "--force":
			force = true
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(taskStartUsage)
			return nil
		default:
			if strings.HasPrefix(args[i], "-") || taskID != "" {
				return fmt.Errorf("unknown task start option %q\n%s", args[i], taskStartUsage)
			}
			taskID = args[i]
		}
	}
	if taskID == "" {
		return fmt.Errorf("%s", taskStartUsage)
	}
	path, err := taskPath(taskID, "/work")
	if err != nil {
		return err
	}
	machine, terminal, err := resolveWorkTerminal(terminalSpec)
	if err != nil {
		return err
	}

	payload := map[string]any{"machine_id": machine.ID, "machine_name": machine.Name, "terminal": terminal}
	if force {
		payload["force"] = true
	}
	body, err := agentDo(http.MethodPost, path, payload)
	if err != nil {
		if httpErr, _ := apiErrorMessage(err); httpErr != nil && httpErr.StatusCode == http.StatusConflict {
			return startConflict(taskID, httpErr, jsonOut)
		}
		return err
	}
	if jsonOut {
		fmt.Println(string(body))
		return nil
	}

	var response struct {
		Task    workTask      `json:"task"`
		Stopped []workRunning `json:"stopped"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("unexpected task start response: %w", err)
	}
	fmt.Printf("Started #%d %q (%s) on %s %s\n", response.Task.ID, response.Task.Title, response.Task.label(), machine.Name, terminal)
	for _, stopped := range response.Stopped {
		fmt.Printf("Took over from %s\n", stopped.chip())
	}
	return nil
}

func startConflict(taskID string, httpErr *httpclient.HTTPError, jsonOut bool) error {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Details struct {
				Running []workRunning `json:"running"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(httpErr.Body, &envelope)
	if jsonOut {
		fmt.Println(strings.TrimSpace(string(httpErr.Body)))
	}
	message := envelope.Error.Message
	if message == "" {
		message = "work on this task is already running elsewhere"
	}
	lines := []string{fmt.Sprintf("task %s: %s", taskID, message)}
	for _, running := range envelope.Error.Details.Running {
		line := "  running: " + running.chip()
		if running.Since != "" {
			line += " since " + running.Since
		}
		lines = append(lines, line)
	}
	lines = append(lines, "  pass --force to take it over")
	return withExitCode(exitConflict, errors.New(strings.Join(lines, "\n")))
}

const taskStopUsage = "usage: missionbase-agent task stop <task-id> [--reason done|manual|reopened|reassigned|terminal_gone|agent_exited] [--json]"

func taskStop(args []string) error {
	taskID, reason, jsonOut := "", "manual", false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--reason":
			if i+1 >= len(args) {
				return fmt.Errorf("--reason requires a value: %s", strings.Join(validStopReasons, ", "))
			}
			reason = args[i+1]
			i++
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(taskStopUsage)
			return nil
		default:
			if strings.HasPrefix(args[i], "-") || taskID != "" {
				return fmt.Errorf("unknown task stop option %q\n%s", args[i], taskStopUsage)
			}
			taskID = args[i]
		}
	}
	if taskID == "" {
		return fmt.Errorf("%s", taskStopUsage)
	}
	if !contains(validStopReasons, reason) {
		return fmt.Errorf("--reason must be one of: %s", strings.Join(validStopReasons, ", "))
	}
	path, err := taskPath(taskID, "/work")
	if err != nil {
		return err
	}
	body, err := agentDo(http.MethodDelete, path, map[string]string{"reason": reason})
	if err != nil {
		return err
	}
	if jsonOut {
		fmt.Println(string(body))
		return nil
	}
	var response struct {
		Task    workTask      `json:"task"`
		Stopped []workRunning `json:"stopped"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("unexpected task stop response: %w", err)
	}
	if len(response.Stopped) == 0 {
		fmt.Printf("Nothing of yours was running on #%s\n", taskID)
		return nil
	}
	for _, stopped := range response.Stopped {
		where := stopped.MachineName
		if where == "" {
			where = stopped.MachineID
		}
		fmt.Printf("Stopped #%s on %s %s (%s)\n", taskID, where, stopped.Terminal, reason)
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// ---- task attention -------------------------------------------------------------

const taskAttentionUsage = `usage: missionbase-agent task attention <task-id> "reason" [--json]
       missionbase-agent task attention <task-id> --body-file PATH [--json]
       missionbase-agent task attention clear <task-id> [--json]`

func taskAttention(args []string) error {
	clear := len(args) > 0 && args[0] == "clear"
	if clear {
		args = args[1:]
	}
	taskID, reason, reasonSet, jsonOut := "", "", false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(taskAttentionUsage)
			return nil
		case "--body-file", "--reason-file":
			if clear {
				return fmt.Errorf("%s is not used with task attention clear", args[i])
			}
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a file path", args[i])
			}
			body, err := readBodyFile(args[i+1])
			if err != nil {
				return err
			}
			reason, reasonSet = body, true
			i++
		default:
			switch {
			case strings.HasPrefix(args[i], "-") && args[i] != "-":
				return fmt.Errorf("unknown task attention option %q\n%s", args[i], taskAttentionUsage)
			case taskID == "":
				taskID = args[i]
			case !clear && !reasonSet:
				reason, reasonSet = args[i], true
			default:
				return fmt.Errorf("unexpected argument %q\n%s", args[i], taskAttentionUsage)
			}
		}
	}
	if taskID == "" {
		return fmt.Errorf("%s", taskAttentionUsage)
	}
	path, err := taskPath(taskID, "/attention")
	if err != nil {
		return err
	}

	var body []byte
	if clear {
		body, err = agentDo(http.MethodDelete, path, nil)
	} else {
		reason = strings.TrimSpace(normalizeAgentAuthoredBody(reason))
		if reason == "" {
			return fmt.Errorf("a reason is required: task attention <task-id> \"reason\" (it is posted as a task comment for a person to answer)")
		}
		body, err = agentDo(http.MethodPost, path, map[string]string{"reason": reason})
	}
	if err != nil {
		return err
	}
	if jsonOut {
		fmt.Println(string(body))
		return nil
	}
	var response struct {
		Task workTask `json:"task"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("unexpected task attention response: %w", err)
	}
	if clear {
		fmt.Printf("#%d %q is %s\n", response.Task.ID, response.Task.Title, response.Task.label())
	} else {
		fmt.Printf("#%d %q needs attention: %s\n", response.Task.ID, response.Task.Title, oneLine(reason, 120))
	}
	return nil
}

// ---- tasks ls ---------------------------------------------------------------------

const tasksLsUsage = "usage: missionbase-agent tasks ls [--all] [--status open|done|canceled|all|STATUS[,STATUS]] [--cwd DIR | --project ID] [--json]"

const maxPages = 1000

func tasksLs(args []string) error {
	var pf projectFlags
	all, status, jsonOut := false, "", false
	for i := 0; i < len(args); i++ {
		if handled, err := pf.parse(args, &i); handled {
			if err != nil {
				return err
			}
			continue
		}
		switch args[i] {
		case "--all", "-a":
			all = true
		case "--status":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return fmt.Errorf("--status requires a value")
			}
			status = strings.TrimSpace(args[i+1])
			i++
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(tasksLsUsage)
			return nil
		default:
			return fmt.Errorf("unknown tasks ls option %q\n%s", args[i], tasksLsUsage)
		}
	}
	if err := pf.validate(); err != nil {
		return err
	}
	if all && (pf.project != "" || pf.cwd != "") {
		return fmt.Errorf("use --all or --cwd/--project, not both")
	}
	query := statusQuery(status)
	var project resolvedProject
	if !all {
		var ok bool
		var err error
		if project, ok, err = resolveProject(pf.project, pf.cwd); err != nil {
			return err
		}
		if !ok {
			return errNoProject()
		}
		query.Set("box_ids", project.BoxID)
	}
	tasks, err := fetchAccessibleTasks(query)
	if err != nil {
		return err
	}

	if all {
		if jsonOut {
			return printJSON(map[string]any{"project": nil, "tasks": rawMessages(tasks)})
		}
		fmt.Printf("All projects (%d tasks)\n", len(tasks))
		printTaskTable(tasks, true)
		return nil
	}
	if jsonOut {
		return printJSON(map[string]any{"project": project, "tasks": rawMessages(tasks)})
	}
	name := project.BoxName
	if name == "" && len(tasks) > 0 && tasks[0].task.Box != nil {
		name = tasks[0].task.Box.Name
	}
	if name == "" {
		name = "Project " + project.BoxID
	}
	fmt.Printf("%s (%d tasks, %s)\n", name, len(tasks), projectSourceLabel(project))
	printTaskTable(tasks, false)
	return nil
}

func projectSourceLabel(p resolvedProject) string {
	switch p.Source {
	case "flag":
		return "from --project"
	case "project_file":
		return "from " + p.Path
	default:
		return "machine default"
	}
}

// statusQuery maps --status to task list filters. Categories and "all" use
// status_category; anything else is passed through as status keys, which the
// server validates (so new statuses need no CLI change). No filter means open.
func statusQuery(status string) url.Values {
	query := url.Values{}
	switch status {
	case "":
	case "all":
		query.Set("status_category", "open,done,canceled")
	case "open", "done", "canceled":
		query.Set("status_category", status)
	default:
		query.Set("status", status)
	}
	return query
}

// fetchAccessibleTasks reads every page of GET /api/v1/tasks/accessible, which
// is already in project, status and box_position order.
func fetchAccessibleTasks(query url.Values) ([]rawTask, error) {
	var all []rawTask
	query.Set("per_page", "500")
	for page := 1; page <= maxPages; page++ {
		query.Set("page", strconv.Itoa(page))
		body, err := agentDo(http.MethodGet, withQuery("/api/v1/tasks/accessible", query), nil)
		if err != nil {
			return nil, err
		}
		var response struct {
			Tasks []json.RawMessage `json:"tasks"`
			Meta  struct {
				HasMore bool `json:"has_more"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("unexpected accessible tasks response: %w", err)
		}
		tasks, err := decodeRawTasks(response.Tasks)
		if err != nil {
			return nil, err
		}
		all = append(all, tasks...)
		if !response.Meta.HasMore || len(response.Tasks) == 0 {
			break
		}
	}
	return all, nil
}

// ---- tasks changes ----------------------------------------------------------------

const tasksChangesUsage = "usage: missionbase-agent tasks changes [--since CURSOR] [--project ID[,ID]] [--json]"

type changesFeed struct {
	tasks   []rawTask
	deleted []int64
	cursor  string
}

func tasksChanges(args []string) error {
	since, boxIDs, jsonOut := "", "", false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--since":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return fmt.Errorf("--since requires a cursor (from a previous tasks changes)")
			}
			since = strings.TrimSpace(args[i+1])
			i++
		case "--project", "--box":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return fmt.Errorf("%s requires a project id", args[i])
			}
			for _, id := range strings.Split(args[i+1], ",") {
				if !isPositiveInteger(strings.TrimSpace(id)) {
					return fmt.Errorf("%s must be project ids separated by commas, got %q", args[i], args[i+1])
				}
			}
			boxIDs = strings.ReplaceAll(args[i+1], " ", "")
			i++
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(tasksChangesUsage)
			return nil
		default:
			return fmt.Errorf("unknown tasks changes option %q\n%s", args[i], tasksChangesUsage)
		}
	}
	feed, err := fetchChanges(since, boxIDs)
	if err != nil {
		return err
	}
	if jsonOut {
		deleted := feed.deleted
		if deleted == nil {
			deleted = []int64{}
		}
		return printJSON(map[string]any{"tasks": rawMessages(feed.tasks), "deleted_task_ids": deleted, "cursor": feed.cursor})
	}
	fmt.Printf("%d changed, %d deleted\n", len(feed.tasks), len(feed.deleted))
	printTaskTable(feed.tasks, true)
	for _, id := range feed.deleted {
		fmt.Printf("#%d\tdeleted\n", id)
	}
	fmt.Printf("cursor: %s\n", feed.cursor)
	return nil
}

// fetchChanges follows the changes feed until has_more is false. Tasks are
// deduped by id (the latest copy wins, in feed order) and tasks reported as
// deleted are dropped.
func fetchChanges(since, boxIDs string) (changesFeed, error) {
	var feed changesFeed
	index := map[int64]int{}
	deleted := map[int64]bool{}
	cursor := since
	for page := 0; page < maxPages; page++ {
		requested := cursor
		query := url.Values{}
		if cursor != "" {
			query.Set("since", cursor)
		}
		if boxIDs != "" {
			query.Set("box_ids", boxIDs)
		}
		query.Set("limit", "500")
		body, err := agentDo(http.MethodGet, withQuery("/api/v1/tasks/changes", query), nil)
		if err != nil {
			return feed, err
		}
		var response struct {
			Tasks          []json.RawMessage `json:"tasks"`
			DeletedTaskIDs []int64           `json:"deleted_task_ids"`
			Cursor         string            `json:"cursor"`
			HasMore        bool              `json:"has_more"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return feed, fmt.Errorf("unexpected changes response: %w", err)
		}
		tasks, err := decodeRawTasks(response.Tasks)
		if err != nil {
			return feed, err
		}
		for _, rt := range tasks {
			if i, seen := index[rt.task.ID]; seen {
				feed.tasks[i] = rt
				continue
			}
			index[rt.task.ID] = len(feed.tasks)
			feed.tasks = append(feed.tasks, rt)
		}
		for _, id := range response.DeletedTaskIDs {
			if !deleted[id] {
				deleted[id] = true
				feed.deleted = append(feed.deleted, id)
			}
		}
		if response.Cursor != "" {
			cursor = response.Cursor
		}
		// Stop when the server says so, or if a page makes no progress.
		if !response.HasMore || (cursor == requested && len(response.Tasks) == 0 && len(response.DeletedTaskIDs) == 0) {
			break
		}
	}
	if len(deleted) > 0 {
		kept := feed.tasks[:0]
		for _, rt := range feed.tasks {
			if !deleted[rt.task.ID] {
				kept = append(kept, rt)
			}
		}
		feed.tasks = kept
	}
	feed.cursor = cursor
	return feed, nil
}

// ---- task next ----------------------------------------------------------------------

const taskNextUsage = "usage: missionbase-agent task next [--cwd DIR | --project ID] [--json]"

// taskNext asks the server for the next task: in the resolved project,
// GET /api/v1/boxes/:id/tasks/next (the first To Do task by position that is
// unassigned or yours and not running); with no project, this agent's next
// assigned task from GET /api/v1/agent/work.
func taskNext(args []string) error {
	var pf projectFlags
	jsonOut := false
	for i := 0; i < len(args); i++ {
		if handled, err := pf.parse(args, &i); handled {
			if err != nil {
				return err
			}
			continue
		}
		switch args[i] {
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(taskNextUsage)
			return nil
		default:
			return fmt.Errorf("unknown task next option %q\n%s", args[i], taskNextUsage)
		}
	}
	if err := pf.validate(); err != nil {
		return err
	}
	project, ok, err := resolveProject(pf.project, pf.cwd)
	if err != nil {
		return err
	}

	path := "/api/v1/agent/work"
	if ok {
		path = "/api/v1/boxes/" + url.PathEscape(project.BoxID) + "/tasks/next"
	}
	next, err := fetchNext(path)
	if err != nil {
		return err
	}

	if jsonOut {
		var projectValue any
		if ok {
			projectValue = project
		}
		var taskValue any
		if next != nil {
			taskValue = next.raw
		}
		return printJSON(map[string]any{"has_work": next != nil, "task": taskValue, "project": projectValue})
	}
	if next == nil {
		if ok {
			fmt.Println("No work here: no To Do task that is unassigned or yours and not already running.")
		} else {
			fmt.Println("No work assigned to you.")
		}
		return nil
	}
	fmt.Printf("#%d %q (%s)\n", next.task.ID, next.task.Title, next.task.label())
	fmt.Printf("Start it with: missionbase-agent task start %d\n", next.task.ID)
	return nil
}

// fetchNext reads a {has_work, task} response.
func fetchNext(path string) (*rawTask, error) {
	body, err := agentDo(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		Task json.RawMessage `json:"task"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("unexpected next task response: %w", err)
	}
	if len(response.Task) == 0 || string(response.Task) == "null" {
		return nil, nil
	}
	tasks, err := decodeRawTasks([]json.RawMessage{response.Task})
	if err != nil {
		return nil, err
	}
	return &tasks[0], nil
}

// ---- project ---------------------------------------------------------------------------

const projectUsage = `usage: missionbase-agent project [show] [--cwd DIR | --project ID] [--json]
       missionbase-agent project default [ID | --clear]`

func projectCommand(args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println(projectUsage)
		return nil
	}
	if len(args) > 0 && args[0] == "default" {
		return projectDefault(args[1:])
	}
	if len(args) > 0 && args[0] == "show" {
		args = args[1:]
	}
	var pf projectFlags
	jsonOut := false
	for i := 0; i < len(args); i++ {
		if handled, err := pf.parse(args, &i); handled {
			if err != nil {
				return err
			}
			continue
		}
		switch args[i] {
		case "--json":
			jsonOut = true
		default:
			return fmt.Errorf("unknown project option %q\n%s", args[i], projectUsage)
		}
	}
	if err := pf.validate(); err != nil {
		return err
	}
	project, ok, err := resolveProject(pf.project, pf.cwd)
	if err != nil {
		return err
	}
	if !ok {
		if jsonOut {
			_ = printJSON(map[string]any{"project": nil})
		}
		return errNoProject()
	}
	if jsonOut {
		return printJSON(map[string]any{"project": project})
	}
	name := ""
	if project.BoxName != "" {
		name = " " + project.BoxName
	}
	fmt.Printf("Project %s%s (%s)\n", project.BoxID, name, projectSourceLabel(project))
	return nil
}

func projectDefault(args []string) error {
	switch {
	case len(args) == 0:
		cfg, err := config.LoadAgent()
		if err != nil {
			return err
		}
		if cfg.DefaultProject == "" {
			fmt.Println("No default project on this machine")
		} else {
			fmt.Println(cfg.DefaultProject)
		}
		return nil
	case len(args) == 1 && args[0] == "--clear":
		path, err := config.SetAgentDefaultProject("")
		if err != nil {
			return err
		}
		fmt.Printf("Cleared the default project in %s\n", path)
		return nil
	case len(args) == 1 && isPositiveInteger(args[0]):
		path, err := config.SetAgentDefaultProject(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("Default project set to %s in %s\n", args[0], path)
		return nil
	default:
		return fmt.Errorf("%s", projectUsage)
	}
}
