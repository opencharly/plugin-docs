package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// generateSoul publishes SOUL.md with its H1 dropped — the same treatment generateVision applies,
// and for the same reason: it is canonical narrative that already exists, and a second,
// "web-friendly" retelling would drift away from the original and quietly start lying.
//
// SOUL.md is the identity of charly and of every agent that works as charly: who charly is, what
// charly stands for, and how charly carries itself. It is deliberately self-contained — it cites
// no other document and carries no links — so this projection is the plainest of all the root
// pages: drop the H1 and publish. There is no link-rewrite map, because the source has no
// repo-relative links to rewrite; a future link would be caught by the whole-site link gate like
// any other, which is the point of publishing it verbatim rather than transcribing it.
func generateSoul(root, out string) error {
	raw, err := os.ReadFile(filepath.Join(root, "SOUL.md"))
	if err != nil {
		return fmt.Errorf("read SOUL.md: %w", err)
	}
	body := string(raw)

	// Drop the source's H1: Starlight renders the frontmatter title as the page heading, so
	// keeping it would show the title twice.
	if strings.HasPrefix(body, "# ") {
		if i := strings.Index(body, "\n"); i >= 0 {
			body = strings.TrimLeft(body[i+1:], "\n")
		}
	}

	return page{
		Path:        "soul.md",
		Title:       "Who We Are",
		Description: "The identity of charly and every charly agent — who charly is, what charly stands for, and how charly carries itself.",
		Body:        body,
	}.write(out)
}
