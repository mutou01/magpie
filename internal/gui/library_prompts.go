package gui

import (
	"encoding/json"
	"net/http"

	"github.com/yetone/magpie/internal/library"
)

func promptRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/library/prompts", func(w http.ResponseWriter, r *http.Request) {
		v, err := library.ReadPrompts()
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, v)
	})
	for _, action := range []string{"save", "check", "remove"} {
		mux.HandleFunc("POST /api/library/prompts/"+action, func(w http.ResponseWriter, r *http.Request) {
			var c library.PromptChange
			if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
				fail(w, err)
				return
			}
			var res *library.Result
			var err error
			if action == "check" {
				// Only an explicit check (passive omitted/false) may load Pi's
				// extensions. A save's preflight sends passive:true.
				v, err := library.CheckPromptContext(r.Context(), c)
				if err != nil {
					fail(w, err)
					return
				}
				writeJSON(w, v)
				return
			}
			if action == "save" {
				res, err = library.SavePrompt(c)
			} else {
				res, err = library.RemovePrompt(c.ID)
			}
			if err != nil {
				fail(w, err)
				return
			}
			v, err := library.ReadPrompts()
			if err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, struct {
				Part    string               `json:"part"`
				Prompts *library.PromptsView `json:"prompts"`
				Result  *library.Result      `json:"result"`
			}{"prompts", v, res})
		})
	}
}
