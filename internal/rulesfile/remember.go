package rulesfile

import (
	"errors"
	"fmt"
	"time"

	"github.com/plavipas/portero/internal/rules"
)

// ErrNoLongerAsks: the rules file changed since the panel was shown and the link is no longer decided by the
// same asking rule, or the chosen target is no longer one of its choices. Nothing is written.
var ErrNoLongerAsks = errors.New("the rules file no longer asks for this link")

// Remember adds a plain rule for the link in req, opened in chosen, before the asking rule (or its group) that
// decided it. label is that decision's rule label (Decision.Rule). It locks lockDir, re-reads the file, routes
// the link again and refuses with ErrNoLongerAsks when the answer changed. It returns the name of the new rule.
func Remember(path, lockDir string, req rules.Request, label string, chosen rules.Target, offloaded func(string) bool, now time.Time) (string, error) {
	host, p, ok := rules.RememberKey(req.URL)
	if !ok {
		return "", errors.New("the link has no path to remember")
	}
	if lockDir != "" {
		unlock, err := Lock(lockDir)
		if err != nil {
			return "", err
		}
		defer unlock()
	}
	if offloaded != nil && offloaded(path) {
		return "", ErrOffloaded
	}
	data, hash, err := Read(path)
	if err != nil {
		return "", fmt.Errorf("read rules file: %w", err)
	}
	r, err := rules.Parse(data)
	if err != nil {
		return "", err
	}
	d := r.Route(req)
	if len(d.Ask) == 0 || d.Rule != label || !containsTarget(d.Ask, chosen) {
		return "", ErrNoLongerAsks
	}
	top := -1
	for i, rule := range r.Rules {
		if d.Group != "" && rule.IsGroup() && rule.Name == d.Group {
			top = i
			break
		}
		if d.Group == "" && !rule.IsGroup() && rules.Label(i, rule) == d.Rule {
			top = i
			break
		}
	}
	if top < 0 {
		return "", ErrNoLongerAsks
	}
	name := UniqueName(r, host+p)
	text, err := InsertBefore(string(data), top, Remembered{Name: name, Host: host, Path: p, Target: chosen}, req, now)
	if err != nil {
		return "", err
	}
	if _, err := Write(path, text, hash, offloaded); err != nil {
		return "", err
	}
	return name, nil
}

func containsTarget(list []rules.Target, t rules.Target) bool {
	for _, x := range list {
		if x == t {
			return true
		}
	}
	return false
}

// ErrNowMatched: a rule matches the link by the time the file is read again (the file changed since the panel
// was shown). Nothing is written.
var ErrNowMatched = errors.New("a rule matches this link now")

// RememberUnmatched appends a host rule for the link in req, which no rule matched when the panel was shown: it
// opens in chosen, or follows the default when followDefault is set. It locks lockDir, re-reads the file and
// writes nothing (ErrNowMatched) when a rule decides the link now. It returns the name of the new rule.
func RememberUnmatched(path, lockDir string, req rules.Request, chosen rules.Target, followDefault bool, offloaded func(string) bool, now time.Time) (string, error) {
	host, ok := rules.RememberHost(req.URL)
	if !ok {
		return "", errors.New("the link has no host to remember")
	}
	if lockDir != "" {
		unlock, err := Lock(lockDir)
		if err != nil {
			return "", err
		}
		defer unlock()
	}
	if offloaded != nil && offloaded(path) {
		return "", ErrOffloaded
	}
	data, hash, err := Read(path)
	if err != nil {
		return "", fmt.Errorf("read rules file: %w", err)
	}
	r, err := rules.Parse(data)
	if err != nil {
		return "", err
	}
	if r.Route(req).Rule != "" {
		return "", ErrNowMatched
	}
	name := UniqueName(r, host)
	text, err := InsertLast(string(data), Remembered{Name: name, Host: host, Target: chosen, FollowDefault: followDefault}, req, now)
	if err != nil {
		return "", err
	}
	if _, err := Write(path, text, hash, offloaded); err != nil {
		return "", err
	}
	return name, nil
}
