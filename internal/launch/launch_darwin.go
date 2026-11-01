//go:build darwin

package launch

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/rules"
)

func open(u *url.URL, t rules.Target, dry bool) (Plan, error) {
	b, _ := browsers.Lookup(t.Browser)
	profile, container, note, err := resolveTarget(b, t)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Browser: b, Profile: profile, Container: container}
	if note != "" {
		plan.Steps = append(plan.Steps, note)
	}
	link := u.String()

	switch b.Kind {
	case browsers.Chromium:
		args := []string{"-b", b.BundleID}
		if profile != "" {
			args = append(args, "-n", "--args", "--profile-directory="+profile)
		}
		return runOpen(plan, dry, append(args, link))
	case browsers.Firefox:
		args := []string{"-a", b.Name}
		if profile != "" {
			args = append(args, "-n", "--args", "-P", profile)
		}
		link, notes := firefoxLink(b, u, t.Profile, container)
		plan.Steps = append(plan.Steps, notes...)
		if link == u.String() {
			plan.Container = ""
		}
		return runOpen(plan, dry, append(args, link))
	case browsers.Dia:
		return runScript(plan, dry, b, diaOpen, link, profile)
	case browsers.Arc:
		return runScript(plan, dry, b, arcOpen, link, profile)
	default:
		flag := "-a"
		if strings.Count(b.Name, ".") >= 2 && !strings.Contains(b.Name, " ") {
			flag = "-b" // a bundle id was given
		}
		return runOpen(plan, dry, []string{flag, b.Name, link})
	}
}

func runOpen(plan Plan, dry bool, args []string) (Plan, error) {
	plan.Steps = append(plan.Steps, "open "+strings.Join(args, " "))
	if dry {
		return plan, nil
	}
	if out, err := exec.Command("/usr/bin/open", args...).CombinedOutput(); err != nil {
		return plan, fmt.Errorf("open %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return plan, nil
}

// runScript drives a single-instance browser: the script makes a tab in the profile through window 1.
// With no window (browser not running) it launches the app with `open -a` (no URL, so nothing lands
// in the wrong profile) and retries the script until a window exists.
func runScript(plan Plan, dry bool, b browsers.Browser, script, link, profile string) (Plan, error) {
	plan.Steps = append(plan.Steps, fmt.Sprintf("osascript %s: new tab in profile %q", b.Name, profile))
	if dry {
		return plan, nil
	}
	_, err := osascript(script, link, profile)
	if err == nil {
		return plan, nil
	}
	if !isNoWindow(err) {
		return plan, fmt.Errorf("%s: %w", b.Name, err)
	}
	plan.Steps = append(plan.Steps, "no window: open -a "+b.Name+", retry")
	if out, err := exec.Command("/usr/bin/open", "-a", b.Name).CombinedOutput(); err != nil {
		return plan, fmt.Errorf("open -a %s: %v: %s", b.Name, err, strings.TrimSpace(string(out)))
	}
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		if _, err = osascript(script, link, profile); err == nil {
			return plan, nil
		}
	}
	return plan, fmt.Errorf("%s: launched, but no window to open the tab in: %w", b.Name, err)
}

func osascript(script string, args ...string) (string, error) {
	cmd := exec.Command("/usr/bin/osascript", append([]string{"-"}, args...)...)
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("osascript: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func isNoWindow(err error) bool {
	s := err.Error()
	return strings.Contains(s, "no window") || strings.Contains(s, "-1728") || strings.Contains(s, "-600")
}

// Dia: window > profiles > tabs. `make new tab at <profile>` opens in that profile and switches to it.
const diaOpen = `on run argv
	set theURL to item 1 of argv
	set theProfile to item 2 of argv
	tell application "Dia"
		if (count of windows) is 0 then error "no window" number 1000
		tell window 1
			if theProfile is "" then
				make new tab with properties {URL:theURL}
			else
				make new tab at (first profile whose name is theProfile) with properties {URL:theURL}
			end if
		end tell
		activate
	end tell
end run`

// Arc: window > spaces (by title) > tabs. Written from Arc's dictionary, not yet verified on a Mac
// with Arc running; a profile in a rule means a space title here.
const arcOpen = `on run argv
	set theURL to item 1 of argv
	set theSpace to item 2 of argv
	tell application "Arc"
		if (count of windows) is 0 then error "no window" number 1000
		tell front window
			if theSpace is "" then
				make new tab with properties {URL:theURL}
			else
				tell (first space whose title is theSpace)
					make new tab with properties {URL:theURL}
				end tell
			end if
		end tell
		activate
	end tell
end run`
