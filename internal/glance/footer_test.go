package glance

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

func renderFooterForTest(t *testing.T, version string, hide bool, custom template.HTML) string {
	t.Helper()

	app := &application{Version: version}
	app.Config.Branding.HideFooter = hide
	app.Config.Branding.CustomFooter = custom

	var output bytes.Buffer
	footerTemplate := mustParseTemplate("footer.html")
	if err := footerTemplate.Execute(&output, templateData{App: app}); err != nil {
		t.Fatal(err)
	}

	return output.String()
}

func TestFooterVersionLinks(t *testing.T) {
	const (
		maintainedTag = "chatarch-v0.2.1"
		commit        = "0123456789abcdef0123456789abcdef01234567"
		upstreamTag   = "v0.8.4"
		brandURL      = "https://github.com/glanceapp/glance"
		maintained    = "https://github.com/ChatArch/glance"
	)

	tests := []struct {
		name          string
		version       string
		hide          bool
		custom        template.HTML
		contains      []string
		absent        []string
		wantLinkCount int
	}{
		{
			name:          "maintained_tag_only",
			version:       maintainedTag,
			contains:      []string{brandURL, maintained + "/releases/tag/" + maintainedTag, ">" + maintainedTag + "</a>"},
			absent:        []string{"/commit/", "glanceapp/glance/releases/tag/" + maintainedTag},
			wantLinkCount: 2,
		},
		{
			name:    "maintained_tag_and_commit",
			version: maintainedTag + "+" + commit,
			contains: []string{
				brandURL,
				maintained + "/releases/tag/" + maintainedTag,
				maintained + "/commit/" + commit,
				">" + maintainedTag + "</a>",
				">0123456</a>",
			},
			absent:        []string{"glanceapp/glance/releases/tag/" + maintainedTag, ">" + commit + "</a>"},
			wantLinkCount: 3,
		},
		{
			name:          "upstream_release",
			version:       upstreamTag,
			contains:      []string{brandURL, brandURL + "/releases/tag/" + upstreamTag, ">" + upstreamTag + "</a>"},
			absent:        []string{maintained, "/commit/"},
			wantLinkCount: 2,
		},
		{
			name:          "development_version",
			version:       "dev",
			contains:      []string{brandURL, "(dev)"},
			absent:        []string{"/releases/tag/", "/commit/"},
			wantLinkCount: 1,
		},
		{
			name:          "unknown_version",
			version:       "nightly",
			contains:      []string{brandURL, "(nightly)"},
			absent:        []string{"/releases/tag/", "/commit/"},
			wantLinkCount: 1,
		},
		{
			name:          "URL_injection",
			version:       maintainedTag + "+" + commit + `"><script src="https://evil.example/x"></script>`,
			contains:      []string{brandURL, "&lt;script"},
			absent:        []string{"/releases/tag/", "/commit/", "<script", `href="https://evil.example`},
			wantLinkCount: 1,
		},
		{
			name:          "hidden_footer",
			version:       maintainedTag + "+" + commit,
			hide:          true,
			absent:        []string{"<footer", "href="},
			wantLinkCount: 0,
		},
		{
			name:          "custom_footer",
			version:       maintainedTag + "+" + commit,
			custom:        template.HTML(`<span id="custom-footer">Custom footer</span>`),
			contains:      []string{`<span id="custom-footer">Custom footer</span>`},
			absent:        []string{">Glance</a>", "/releases/tag/", "/commit/"},
			wantLinkCount: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := renderFooterForTest(t, test.version, test.hide, test.custom)
			for _, value := range test.contains {
				if !strings.Contains(output, value) {
					t.Errorf("missing %q in footer:\n%s", value, output)
				}
			}
			for _, value := range test.absent {
				if strings.Contains(output, value) {
					t.Errorf("unexpected %q in footer:\n%s", value, output)
				}
			}
			if got := strings.Count(output, "href="); got != test.wantLinkCount {
				t.Errorf("link count = %d, want %d; footer:\n%s", got, test.wantLinkCount, output)
			}
		})
	}
}
