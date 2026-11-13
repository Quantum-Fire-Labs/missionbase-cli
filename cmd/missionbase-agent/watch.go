package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
)

// ---- project watch -------------------------------------------------------------
//
// A watch signs up one hc terminal on this machine for a project's activity.
// The missionbase-herdr bridge reads the file and, when that terminal is idle,
// types a short digest of the owner's new tasks, replies, answers and status
// changes, and of mentions of the watching agent. Nothing is stored in
// Missionbase. Each project has at most one watch: signing up again replaces it.

const projectWatchUsage = `usage: missionbase-agent project watch --terminal tN [--cwd DIR | --project ID] [--json]
       missionbase-agent project watch --off [--cwd DIR | --project ID] [--json]
       missionbase-agent project watch [--cwd DIR | --project ID] [--json]`

const watchFileVersion = 1

// projectWatch is one entry in the watches file, which the bridge reads.
type projectWatch struct {
	ProjectID   int64  `json:"project_id"`
	ProjectName string `json:"project_name,omitempty"`
	Terminal    string `json:"terminal"`
	AgentSlug   string `json:"agent_slug"`
	Since       string `json:"since"`
}

type watchFile struct {
	Version int            `json:"version"`
	Watches []projectWatch `json:"watches"`
}

func readWatchFile(path string) (watchFile, error) {
	file := watchFile{Version: watchFileVersion}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return file, err
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return file, fmt.Errorf("read %s: %w", path, err)
	}
	return file, nil
}

// writeWatchFile replaces the file in one rename, so the bridge never reads half of it.
func writeWatchFile(path string, file watchFile) error {
	file.Version = watchFileVersion
	if file.Watches == nil {
		file.Watches = []projectWatch{}
	}
	sort.Slice(file.Watches, func(i, j int) bool { return file.Watches[i].ProjectID < file.Watches[j].ProjectID })
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".watches-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (f watchFile) find(projectID int64) (projectWatch, bool) {
	for _, w := range f.Watches {
		if w.ProjectID == projectID {
			return w, true
		}
	}
	return projectWatch{}, false
}

func (f *watchFile) remove(projectID int64) {
	kept := f.Watches[:0]
	for _, w := range f.Watches {
		if w.ProjectID != projectID {
			kept = append(kept, w)
		}
	}
	f.Watches = kept
}

func projectWatchCommand(args []string) error {
	var pf projectFlags
	terminal, off, jsonOut := "", false, false
	for i := 0; i < len(args); i++ {
		if handled, err := pf.parse(args, &i); handled {
			if err != nil {
				return err
			}
			continue
		}
		switch args[i] {
		case "--terminal":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return fmt.Errorf("--terminal requires a terminal address like t7")
			}
			terminal = strings.TrimSpace(args[i+1])
			i++
		case "--off":
			off = true
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(projectWatchUsage)
			return nil
		default:
			return fmt.Errorf("unknown project watch option %q\n%s", args[i], projectWatchUsage)
		}
	}
	if err := pf.validate(); err != nil {
		return err
	}
	if off && terminal != "" {
		return fmt.Errorf("use only one of --terminal or --off")
	}
	if terminal != "" {
		if err := validateWatchTerminal(terminal); err != nil {
			return err
		}
	}

	path := config.HerdrWatchesPath()
	file, err := readWatchFile(path)
	if err != nil {
		return err
	}
	project, ok, err := resolveProject(pf.project, pf.cwd)
	if err != nil {
		return err
	}
	if !ok {
		if terminal != "" || off {
			return errNoProject()
		}
		return showAllWatches(file, path, jsonOut)
	}
	projectID, _ := strconv.ParseInt(project.ID, 10, 64)

	switch {
	case terminal != "":
		return watchProject(&file, path, project, projectID, terminal, jsonOut)
	case off:
		previous, had := file.find(projectID)
		if had {
			file.remove(projectID)
			if err := writeWatchFile(path, file); err != nil {
				return err
			}
		}
		if jsonOut {
			return printJSON(map[string]any{"watch": nil, "stopped": watchOrNil(previous, had), "path": path})
		}
		if had {
			fmt.Printf("Stopped watching project %d (was %s)\n", projectID, previous.Terminal)
		} else {
			fmt.Printf("Project %d isn't watched on this machine\n", projectID)
		}
		return nil
	default:
		w, had := file.find(projectID)
		if jsonOut {
			return printJSON(map[string]any{"watch": watchOrNil(w, had), "path": path})
		}
		if had {
			fmt.Println(describeWatch(w))
		} else {
			fmt.Printf("Project %s isn't watched on this machine\n", project.ID)
		}
		return nil
	}
}

func watchProject(file *watchFile, path string, project resolvedProject, projectID int64, terminal string, jsonOut bool) error {
	cfg, err := config.LoadAgent()
	if err != nil {
		return err
	}
	if cfg.AgentSlug == "" {
		return fmt.Errorf("agent slug is not set; run `missionbase-agent use <slug>` in this directory or set MISSIONBASE_AGENT_SLUG")
	}
	info, err := fetchProject(project.ID) // the agent must be able to see the project
	if err != nil {
		return err
	}
	previous, had := file.find(projectID)
	w := projectWatch{
		ProjectID:   projectID,
		ProjectName: info.Name,
		Terminal:    terminal,
		AgentSlug:   cfg.AgentSlug,
		Since:       time.Now().UTC().Format(time.RFC3339),
	}
	file.remove(projectID)
	file.Watches = append(file.Watches, w)
	if err := writeWatchFile(path, *file); err != nil {
		return err
	}
	if jsonOut {
		return printJSON(map[string]any{"watch": w, "replaced": watchOrNil(previous, had), "path": path})
	}
	fmt.Println(describeWatch(w))
	if had && previous.Terminal != terminal {
		fmt.Printf("Replaced %s, which no longer gets this project's activity\n", previous.Terminal)
	}
	fmt.Println("The Herdr bridge types a digest into that terminal when it's idle. Stop with: project watch --off")
	return nil
}

func showAllWatches(file watchFile, path string, jsonOut bool) error {
	if jsonOut {
		return printJSON(map[string]any{"watches": file.Watches, "path": path})
	}
	if len(file.Watches) == 0 {
		fmt.Println("No project is watched on this machine")
		return nil
	}
	for _, w := range file.Watches {
		fmt.Println(describeWatch(w))
	}
	return nil
}

func describeWatch(w projectWatch) string {
	name := ""
	if w.ProjectName != "" {
		name = " " + w.ProjectName
	}
	return fmt.Sprintf("Project %d%s is watched by %s as %s (since %s)", w.ProjectID, name, w.Terminal, w.AgentSlug, w.Since)
}

func watchOrNil(w projectWatch, ok bool) any {
	if !ok {
		return nil
	}
	return w
}

// validateWatchTerminal accepts a terminal on this machine (tN). The watches
// file is local, so only this machine's bridge can type into the terminal.
func validateWatchTerminal(terminal string) error {
	if strings.Contains(terminal, "/") {
		return fmt.Errorf("--terminal %q: a watch is kept on this machine; run project watch on the machine the terminal runs on and pass only tN", terminal)
	}
	if strings.ContainsAny(terminal, " \t") || len(terminal) > 100 {
		return fmt.Errorf("--terminal %q is not a terminal address like t7", terminal)
	}
	return nil
}
