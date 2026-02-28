package admin

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
)

//go:embed templates/*.html templates/partials/*.html
var templateFiles embed.FS

type TemplateData struct {
	Title     string
	Nav       string
	Content   any
	CSRFToken string
	User      *generated.User
	Flash     string
}

type templateRegistry struct {
	templates map[string]*template.Template
}

func loadTemplates() (*templateRegistry, error) {
	funcMap := template.FuncMap{
		"formatTime": func(t pgtype.Timestamptz) string {
			if !t.Valid {
				return "N/A"
			}
			return t.Time.Format("2006-01-02 15:04:05 UTC")
		},
		"formatTimeShort": func(t pgtype.Timestamptz) string {
			if !t.Valid {
				return "N/A"
			}
			return t.Time.Format("Jan 02, 2006")
		},
		"timeAgo": func(t pgtype.Timestamptz) string {
			if !t.Valid {
				return "never"
			}
			d := time.Since(t.Time)
			switch {
			case d < time.Minute:
				return "just now"
			case d < time.Hour:
				m := int(d.Minutes())
				if m == 1 {
					return "1 minute ago"
				}
				return fmt.Sprintf("%d minutes ago", m)
			case d < 24*time.Hour:
				h := int(d.Hours())
				if h == 1 {
					return "1 hour ago"
				}
				return fmt.Sprintf("%d hours ago", h)
			default:
				days := int(d.Hours() / 24)
				if days == 1 {
					return "1 day ago"
				}
				return fmt.Sprintf("%d days ago", days)
			}
		},
		"uuidStr": func(id pgtype.UUID) string {
			if !id.Valid {
				return ""
			}
			return uuid.UUID(id.Bytes).String()
		},
		"csrfField": func(token string) template.HTML {
			return template.HTML(fmt.Sprintf(`<input type="hidden" name="csrf_token" value="%s">`, template.HTMLEscapeString(token)))
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"seq": func(start, end int) []int {
			s := make([]int, 0, end-start+1)
			for i := start; i <= end; i++ {
				s = append(s, i)
			}
			return s
		},
		"isValid": func(t pgtype.Timestamptz) bool {
			return t.Valid
		},
		"textValue": func(t pgtype.Text) string {
			if !t.Valid {
				return ""
			}
			return t.String
		},
	}

	pages := []string{
		"login",
		"dashboard",
		"users",
		"user_detail",
		"spaces",
		"space_detail",
		"audit",
	}

	reg := &templateRegistry{
		templates: make(map[string]*template.Template),
	}

	for _, page := range pages {
		var t *template.Template
		var err error

		if page == "login" {
			t, err = template.New("login.html").Funcs(funcMap).ParseFS(templateFiles,
				"templates/login.html",
			)
		} else {
			t, err = template.New("layout.html").Funcs(funcMap).ParseFS(templateFiles,
				"templates/layout.html",
				fmt.Sprintf("templates/%s.html", page),
			)
		}
		if err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", page, err)
		}
		reg.templates[page] = t
	}

	// Partials (rendered without layout)
	partials := []string{"stats"}
	for _, partial := range partials {
		t, err := template.New(partial + ".html").Funcs(funcMap).ParseFS(templateFiles,
			fmt.Sprintf("templates/partials/%s.html", partial),
		)
		if err != nil {
			return nil, fmt.Errorf("parsing partial %s: %w", partial, err)
		}
		reg.templates["partial:"+partial] = t
	}

	return reg, nil
}

func (tr *templateRegistry) render(w http.ResponseWriter, r *http.Request, name string, data TemplateData) {
	if data.Flash == "" {
		data.Flash = r.URL.Query().Get("flash")
	}

	t, ok := tr.templates[name]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		http.Error(w, "template render error", http.StatusInternalServerError)
	}
}

func (tr *templateRegistry) renderPartial(w io.Writer, name string, data any) error {
	t, ok := tr.templates["partial:"+name]
	if !ok {
		return fmt.Errorf("partial template %q not found", name)
	}
	return t.Execute(w, data)
}
