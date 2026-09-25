package router

import (
	"fmt"
	"os/exec"
	"strings"
)

// downloadViaModelscope shells out to the modelscope Python SDK to download
// a GGUF model (q4_k_m variant + config). Returns the local snapshot path.
//
// We use the SDK (not raw HTTP) because modelscope handles resumability +
// mirrors. If the SDK isn't installed, returns an error instructing the user
// to `pip install modelscope`.
func downloadViaModelscope(modelID string) (string, error) {
	// Verify modelscope is importable.
	if err := exec.Command("python3", "-c", "import modelscope").Run(); err != nil {
		return "", fmt.Errorf("modelscope SDK not installed: run `pip install modelscope` (got: %v)", err)
	}
	// Download via a small python script. We capture the printed path.
	script := fmt.Sprintf(`
from modelscope import snapshot_download
import sys
loc = snapshot_download(%q, allow_patterns=["*Q4_K_M*", "*q4_k_m*", "*.json", "params", "tokenizer*"])
print(loc)
`, modelID)
	out, err := exec.Command("python3", "-c", script).Output()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		msg := err.Error()
		if ok {
			msg = string(ee.Stderr)
		}
		return "", fmt.Errorf("modelscope download failed: %s\n%s", msg, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}
