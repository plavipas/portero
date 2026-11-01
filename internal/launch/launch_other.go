//go:build !darwin

package launch

import (
	"fmt"
	"net/url"
	"runtime"

	"github.com/plavipas/portero/internal/rules"
)

func open(u *url.URL, t rules.Target, dry bool) (Plan, error) {
	return Plan{}, fmt.Errorf("launch: %s is not supported yet", runtime.GOOS)
}
