package actions

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/iGGUss/goka"
	"github.com/iGGUss/goka/web/templates"

	"github.com/gorilla/mux"
)

type Actor interface {
	RunAction(ctx context.Context, value string) error
	Description() string
}

// Server is a provides HTTP routes for querying the group table.
type Server struct {
	log goka.Logger
	m   sync.RWMutex

	basePath string
	loader   templates.Loader
	actions  map[string]*action
}

// NewServer creates a server with the given options.
func NewServer(basePath string, router *mux.Router, opts ...Option) *Server {
	srv := &Server{
		log:      goka.DefaultLogger(),
		basePath: basePath,
		loader:   &templates.BinLoader{},
		actions:  make(map[string]*action),
	}

	for _, opt := range opts {
		opt(srv)
	}

	sub := router.PathPrefix(basePath).Subrouter()
	sub.HandleFunc("/", srv.index)
	sub.HandleFunc("", srv.index)
	sub.HandleFunc("/start/{action:.*}", srv.startAction).Methods("POST")
	sub.HandleFunc("/stop/{action:.*}", srv.stopAction).Methods("POST")

	return srv
}

func (s *Server) startAction(w http.ResponseWriter, r *http.Request) {

	actionName := mux.Vars(r)["action"]
	action := s.actions[actionName]
	switch {
	case action == nil:
		s.redirect(w, r, fmt.Sprintf("Action '%s' not found", actionName))

	case action.IsRunning():
		s.redirect(w, r, "action already running.")
	default:
		action.Start(r.FormValue("value"))
		s.redirect(w, r, "")
	}
}

func (s *Server) stopAction(w http.ResponseWriter, r *http.Request) {
	actionName := mux.Vars(r)["action"]
	action := s.actions[actionName]
	switch {
	case action == nil:
		s.redirect(w, r, fmt.Sprintf("Action '%s' not found", actionName))

	case !action.IsRunning():
		s.redirect(w, r, "action is not running.")
	default:
		action.Stop()
		s.redirect(w, r, "")
	}
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, errMessage string) {
	var path = s.basePath
	if errMessage != "" {
		path += "?error=" + errMessage
	}

	http.Redirect(w, r, path, http.StatusFound)
}

func (s *Server) BasePath() string {
	return s.basePath
}

func (s *Server) sortedActions() []*action {
	s.m.RLock()
	defer s.m.RUnlock()
	var actions []*action
	for _, action := range s.actions {
		actions = append(actions, action)
	}
	sort.Slice(actions, func(i, j int) bool {
		return strings.Compare(actions[i].name, actions[j].name) < 0
	})
	return actions
}

// AttachSource attaches a new source to the query server.
func (s *Server) AttachAction(name string, actor Actor) error {
	s.m.Lock()
	defer s.m.Unlock()
	if _, exists := s.actions[name]; exists {
		return fmt.Errorf("source with name '%s' is already attached", name)
	}
	s.actions[name] = &action{
		name:  name,
		actor: actor,
	}
	return nil
}

func (s *Server) AttachFuncAction(name string, description string, actor func(ctx context.Context, value string) error) error {
	return s.AttachAction(name, FuncActor(description, actor))
}

func (s *Server) executeTemplate(w http.ResponseWriter, params map[string]interface{}) {
	tmpl, err := templates.LoadTemplates(append(templates.BaseTemplates, "web/templates/actions/index.go.html")...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	params["menu_title"] = "menu title"
	params["base_path"] = s.basePath

	if err := tmpl.Execute(w, params); err != nil {
		s.log.Printf("error executing query template: %v", err)
	}
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	params := map[string]interface{}{
		"page_title": "Actions",
		"actions":    s.sortedActions(),
		"error":      r.URL.Query()["error"],
	}

	s.executeTemplate(w, params)
}
