package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/vatsimnerd/github-artifact-fetcher/fetcher"
)

type (
	GithubEventData struct {
		RunID int `json:"run_id"`
	}

	GithubEvent struct {
		Event      string          `json:"event"`
		Repository string          `json:"repository"`
		Commit     string          `json:"commit"`
		Ref        string          `json:"ref"`
		Head       string          `json:"head"`
		Workflow   string          `json:"workflow"`
		RequestID  string          `json:"requestID"`
		Data       GithubEventData `json:"data"`
	}

	ErrorResponse struct {
		Error string `json:"error"`
	}

	MatchResponse struct {
		MatchesCount int `json:"matches_count"`
	}
)

func (ge GithubEvent) String() string {
	return fmt.Sprintf("<GithubEvent event=\"%s\" repo=\"%s\" commit=\"%s\""+
		" ref=\"%s\" head=\"%s\" workflow=\"%s\" requestID=\"%s\" runID=%d",
		ge.Event,
		ge.Repository,
		ge.Commit,
		ge.Ref,
		ge.Head,
		ge.Workflow,
		ge.RequestID,
		ge.Data.RunID,
	)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// TODO check headers X-Hub-Signature[-256]
	defer r.Body.Close()

	log.Debug("reading request body")
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		sendError(w, 400, fmt.Sprintf("can't read request body: %s", err))
		return
	}

	log.Debug("parsing github event data")
	var ge GithubEvent
	err = json.Unmarshal(body, &ge)
	if err != nil {
		log.WithError(err).Error("error parsing body")
		sendError(w, 400, fmt.Sprintf("error parsing body: %v", err))
		return
	}

	matches := 0

	log.WithField("github_event", ge.String()).Info("received github event")
	for _, artifact := range s.cfg.Artifacts {
		if artifact.Filter.Event != "" && ge.Event != artifact.Filter.Event {
			// event filter mismatch
			continue
		}
		if artifact.Filter.Workflow != "" && ge.Workflow != artifact.Filter.Workflow {
			// workflow filter mismatch
			continue
		}
		if artifact.Repo != ge.Repository {
			// wrong repo
			continue
		}
		s.fetcher.Enqueue(fetcher.FetchTask{RunID: ge.Data.RunID, ArtifactCfg: artifact})
		matches++
	}
	log.WithField("github_event", ge.String()).Infof("found %d matches", matches)

	data, err := json.Marshal(&MatchResponse{MatchesCount: matches})
	if err != nil {
		log.WithError(err).Debug("sending error response to github")
		sendError(w, 500, fmt.Sprintf("can't stringify response: %v", err))
		return
	}

	log.Debug("sending successful response to github")
	w.Write(data)
}

func sendError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	w.Header().Add("Content-Type", "application/json")
	data, err := json.Marshal(&ErrorResponse{Error: message})
	if err != nil {
		log.WithError(err).Error("error marshaling error response")
		return
	}
	_, err = w.Write(data)
	if err != nil {
		log.WithError(err).Error("error sending error response")
	}
}
