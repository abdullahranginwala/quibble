package cli

import (
	"time"

	"github.com/abdullahranginwala/quibble/pkg/comment"
)

// The JSON views freeze the machine-readable field names to the thread's
// frontmatter (yaml) keys, so M6's agent skill can depend on them (see
// plan/06-m4-cli.md, "field names frozen from here on").

type anchorJSON struct {
	Exact    string   `json:"exact"`
	Prefix   string   `json:"prefix"`
	Suffix   string   `json:"suffix"`
	Heading  []string `json:"heading,omitempty"`
	Position int      `json:"position"`
}

type replyJSON struct {
	Author string `json:"author"`
	Time   string `json:"time"`
	Body   string `json:"body"`
}

type threadJSON struct {
	ID         string      `json:"id"`
	Doc        string      `json:"doc"`
	Status     string      `json:"status"`
	Created    string      `json:"created"`
	Author     string      `json:"author"`
	Anchor     anchorJSON  `json:"anchor"`
	Body       string      `json:"body"`
	Replies    []replyJSON `json:"replies,omitempty"`
	ResolvedBy string      `json:"resolved_by,omitempty"`
	ResolvedAt *string     `json:"resolved_at,omitempty"`
}

func toThreadJSON(t *comment.Thread) threadJSON {
	tj := threadJSON{
		ID:      t.ID,
		Doc:     t.Doc,
		Status:  string(t.Status),
		Created: t.Created.Format(time.RFC3339),
		Author:  t.Author,
		Anchor: anchorJSON{
			Exact:    t.Anchor.Exact,
			Prefix:   t.Anchor.Prefix,
			Suffix:   t.Anchor.Suffix,
			Heading:  t.Anchor.Heading,
			Position: t.Anchor.Position,
		},
		Body:       t.Body,
		ResolvedBy: t.ResolvedBy,
	}
	for _, r := range t.Replies {
		tj.Replies = append(tj.Replies, replyJSON{
			Author: r.Author,
			Time:   r.Time.Format(time.RFC3339),
			Body:   r.Body,
		})
	}
	if t.ResolvedAt != nil {
		s := t.ResolvedAt.Format(time.RFC3339)
		tj.ResolvedAt = &s
	}
	return tj
}
