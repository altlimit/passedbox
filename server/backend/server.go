package main

import (
	"io/fs"
	"net/http"
	"regexp"

	rs "github.com/altlimit/restruct"
	"passedbox.com/api"
)

type Server struct {
	Task  *Task
	ApiV1 api.VaultAPI `route:"api/v1"`
}

func NewServer(vaultAPI api.VaultAPI) *Server {
	return &Server{
		ApiV1: vaultAPI,
	}
}

// Index serves the SPA shell. Auth is handled client-side by the Vue router.
func (s *Server) Any(w http.ResponseWriter, r *http.Request) *rs.Render {
	return &rs.Render{Path: "index.html"}
}

func (s *Server) Writer() rs.ResponseWriter {
	sub, _ := fs.Sub(publicFS, "public")
	return &rs.View{
		FS:      sub,
		Skips:   regexp.MustCompile("^layout"),
		Layouts: []string{"layout/*.html"},
		Error:   "error.html",
	}
}
