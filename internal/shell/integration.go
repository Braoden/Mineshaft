// ABOUTME: Shell integration installation and removal for Mineshaft.
// ABOUTME: Manages the shell hook in RC files with safe block markers.

package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/mineshaft/internal/state"
)

const (
	markerStart = "# --- Mineshaft Integration (managed by ms) ---"
	markerEnd   = "# --- End Mineshaft ---"
)

func hookSourceLine() string {
	return fmt.Sprintf(`[[ -f "%s/shell-hook.sh" ]] && source "%s/shell-hook.sh"`,
		state.ConfigDir(), state.ConfigDir())
}

func Install() error {
	shell := DetectShell()
	rcPath := RCFilePath(shell)

	if err := writeHookScript(); err != nil {
		return fmt.Errorf("writing hook script: %w", err)
	}

	if err := addToRCFile(rcPath); err != nil {
		return fmt.Errorf("updating %s: %w", rcPath, err)
	}

	return state.SetShellIntegration(shell)
}

func Remove() error {
	shell := DetectShell()
	rcPath := RCFilePath(shell)

	if err := removeFromRCFile(rcPath); err != nil {
		return fmt.Errorf("updating %s: %w", rcPath, err)
	}

	hookPath := filepath.Join(state.ConfigDir(), "shell-hook.sh")
	if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing hook script: %w", err)
	}

	return nil
}

func DetectShell() string {
	shell := os.Getenv("SHELL")
	if strings.HasSuffix(shell, "zsh") {
		return "zsh"
	}
	if strings.HasSuffix(shell, "bash") {
		return "bash"
	}
	return "zsh"
}

func RCFilePath(shell string) string {
	home, _ := os.UserHomeDir()
	switch shell {
	case "bash":
		return filepath.Join(home, ".bashrc")
	default:
		return filepath.Join(home, ".zshrc")
	}
}

func writeHookScript() error {
	dir := state.ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	hookPath := filepath.Join(dir, "shell-hook.sh")
	return os.WriteFile(hookPath, []byte(shellHookScript), 0644)
}

func addToRCFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)

	if strings.Contains(content, markerStart) {
		return updateRCFile(path, content)
	}

	block := fmt.Sprintf("\n%s\n%s\n%s\n", markerStart, hookSourceLine(), markerEnd)

	if len(data) > 0 {
		backupPath := path + ".mineshaft-backup"
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("writing backup: %w", err)
		}
	}

	return os.WriteFile(path, []byte(content+block), 0644)
}

func removeFromRCFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)

	startIdx := strings.Index(content, markerStart)
	if startIdx == -1 {
		return nil
	}

	endIdx := strings.Index(content[startIdx:], markerEnd)
	if endIdx == -1 {
		return nil
	}
	endIdx += startIdx + len(markerEnd)

	if endIdx < len(content) && content[endIdx] == '\n' {
		endIdx++
	}

	if startIdx > 0 && content[startIdx-1] == '\n' {
		startIdx--
	}

	newContent := content[:startIdx] + content[endIdx:]
	return os.WriteFile(path, []byte(newContent), 0644)
}

func updateRCFile(path, content string) error {
	startIdx := strings.Index(content, markerStart)
	endIdx := strings.Index(content[startIdx:], markerEnd)
	if endIdx == -1 {
		return fmt.Errorf("malformed Mineshaft block in %s", path)
	}
	endIdx += startIdx + len(markerEnd)

	block := fmt.Sprintf("%s\n%s\n%s", markerStart, hookSourceLine(), markerEnd)
	newContent := content[:startIdx] + block + content[endIdx:]

	return os.WriteFile(path, []byte(newContent), 0644)
}

var shellHookScript = `#!/bin/zsh
# Mineshaft Shell Integration
# Installed by: ms install --shell
# Location: ~/.config/mineshaft/shell-hook.sh

_mineshaft_enabled() {
    [[ -n "$MINESHAFT_DISABLED" ]] && return 1
    [[ -n "$MINESHAFT_ENABLED" ]] && return 0
    local state_file="$HOME/.local/state/mineshaft/state.json"
    [[ -f "$state_file" ]] && grep -q '"enabled":\s*true' "$state_file" 2>/dev/null
}

_mineshaft_ignored() {
    local dir="$PWD"
    while [[ "$dir" != "/" ]]; do
        [[ -f "$dir/.mineshaft-ignore" ]] && return 0
        dir="$(dirname "$dir")"
    done
    return 1
}

_mineshaft_already_asked() {
    local repo_root="$1"
    local asked_file="$HOME/.cache/mineshaft/asked-repos"
    [[ -f "$asked_file" ]] && grep -qF "$repo_root" "$asked_file" 2>/dev/null
}

_mineshaft_mark_asked() {
    local repo_root="$1"
    local asked_file="$HOME/.cache/mineshaft/asked-repos"
    mkdir -p "$(dirname "$asked_file")"
    echo "$repo_root" >> "$asked_file"
}

_mineshaft_offer_add() {
    local repo_root="$1"

    # Offer-to-add is OPT-IN. By default Mineshaft stays silent in your shells
    # and only sets MS_TOWN_ROOT/MS_RIG when you are inside a known rig. To be
    # prompted to add unrecognized git repos, set MINESHAFT_OFFER_ADD=1. Add a
    # repo any time with 'ms rig quick-add'.
    [[ "${MINESHAFT_OFFER_ADD:-}" == "1" ]] || return 0
    [[ "${MINESHAFT_DISABLE_OFFER_ADD:-}" == "1" ]] && return 0
    _mineshaft_already_asked "$repo_root" && return 0

    [[ -t 0 ]] || return 0

    local repo_name
    repo_name=$(basename "$repo_root")

    # Record that we asked BEFORE reading the answer. If the prompt is
    # interrupted (Ctrl-C) we must not re-offer on the next prompt -- otherwise
    # an interrupted read loops forever (e.g. across restored terminal sessions).
    _mineshaft_mark_asked "$repo_root"

    echo ""
    echo -n "Add '$repo_name' to Mineshaft? [y/N/never] "
    read -r response </dev/tty || response=""

    case "$response" in
        y|Y|yes)
            echo "Adding to Mineshaft..."
            local output
            output=$(ms rig quick-add "$repo_root" --yes 2>&1)
            local exit_code=$?
            echo "$output"

            if [[ $exit_code -eq 0 ]]; then
                local crew_path
                crew_path=$(echo "$output" | grep "^MS_CREW_PATH=" | cut -d= -f2)
                if [[ -n "$crew_path" && -d "$crew_path" ]]; then
                    echo ""
                    echo "Switching to crew workspace..."
                    cd "$crew_path" || true
                    # Re-run hook to set MS_TOWN_ROOT and MS_RIG
                    _mineshaft_hook
                fi
            fi
            ;;
        never)
            touch "$repo_root/.mineshaft-ignore"
            echo "Created .mineshaft-ignore - won't ask again for this repo."
            ;;
        *)
            echo "Skipped. Run 'ms rig quick-add' later to add manually."
            ;;
    esac
}

_mineshaft_hook() {
    local previous_exit_status=$?

    _mineshaft_enabled || {
        unset MS_TOWN_ROOT MS_RIG
        return $previous_exit_status
    }

    _mineshaft_ignored && {
        unset MS_TOWN_ROOT MS_RIG
        return $previous_exit_status
    }

    if ! git rev-parse --git-dir &>/dev/null; then
        unset MS_TOWN_ROOT MS_RIG
        return $previous_exit_status
    fi

    local repo_root
    repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
        unset MS_TOWN_ROOT MS_RIG
        return $previous_exit_status
    }

    local cache_file="$HOME/.cache/mineshaft/rigs.cache"
    if [[ -f "$cache_file" ]]; then
        local cached
        cached=$(grep "^${repo_root}:" "$cache_file" 2>/dev/null)
        if [[ -n "$cached" ]]; then
            eval "${cached#*:}"
            return $previous_exit_status
        fi
    fi

    if command -v ms &>/dev/null; then
        local detect_output
        detect_output=$(ms rig detect "$repo_root" 2>/dev/null)
        eval "$detect_output"

        if [[ -n "$MS_TOWN_ROOT" ]]; then
            (ms rig detect --cache "$repo_root" &>/dev/null &)
        elif [[ -n "$_MINESHAFT_PWD_CHANGED" ]]; then
            _mineshaft_offer_add "$repo_root"
            unset _MINESHAFT_PWD_CHANGED
        fi
    fi

    return $previous_exit_status
}

# zsh chpwd hook: fires only when the working directory actually changes.
_mineshaft_chpwd_hook() {
    _MINESHAFT_PWD_CHANGED=1
    _mineshaft_hook
}

# bash has no chpwd; emulate it from PROMPT_COMMAND by tracking $PWD so the
# add-offer is only considered when the directory actually changed -- not on
# every prompt redraw (which previously re-prompted on every command).
_mineshaft_bash_prompt() {
    if [[ "$PWD" != "${_MINESHAFT_LAST_PWD-}" ]]; then
        _MINESHAFT_LAST_PWD="$PWD"
        _MINESHAFT_PWD_CHANGED=1
    fi
    _mineshaft_hook
}

case "${SHELL##*/}" in
    zsh)
        autoload -Uz add-zsh-hook
        add-zsh-hook chpwd _mineshaft_chpwd_hook
        add-zsh-hook precmd _mineshaft_hook
        ;;
    bash)
        # Seed last-seen PWD so a fresh shell doesn't count its first prompt as
        # a directory change (matches zsh, which only fires chpwd on real cd).
        _MINESHAFT_LAST_PWD="$PWD"
        if [[ ";${PROMPT_COMMAND[*]:-};" != *";_mineshaft_bash_prompt;"* ]]; then
            PROMPT_COMMAND="_mineshaft_bash_prompt${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
        fi
        ;;
esac

_mineshaft_hook
`
