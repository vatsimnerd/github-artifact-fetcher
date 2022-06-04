package fetcher

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vatsimnerd/github-artifact-fetcher/config"
)

type (
	Artifact struct {
		ID                 int       `json:"id"`
		NodeID             string    `json:"node_id"`
		Name               string    `json:"name"`
		Size               int       `json:"size_in_bytes"`
		URL                string    `json:"url"`
		ArchiveDownloadURL string    `json:"archive_download_url"`
		Expired            bool      `json:"expired"`
		CreatedAt          time.Time `json:"created_at"`
		UpdatedAt          time.Time `json:"updated_at"`
		ExpiresAt          time.Time `json:"expires_at"`
	}

	GithubArtifactList struct {
		TotalCount int        `json:"total_count"`
		Artifacts  []Artifact `json:"artifacts"`
	}

	Fetcher struct {
		cfg  *config.Config
		cli  *http.Client
		q    chan FetchTask
		stop chan struct{}
	}

	FetchTask struct {
		RunID       int
		ArtifactCfg config.ArtifactConfig
	}
)

var (
	log = logrus.WithField("module", "fetcher")
)

func (a *Artifact) Environ() []string {
	env := make([]string, 0, 8)
	env = append(env, fmt.Sprintf("GITHUB_ARTIFACT_ID=%d", a.ID))
	env = append(env, fmt.Sprintf("GITHUB_ARTIFACT_NAME=%s", a.Name))
	env = append(env, fmt.Sprintf("GITHUB_ARTIFACT_SIZE=%d", a.Size))
	env = append(env, fmt.Sprintf("GITHUB_ARTIFACT_URL=%s", a.URL))
	env = append(env, fmt.Sprintf("GITHUB_ARTIFACT_DOWNLOAD_URL=%s", a.ArchiveDownloadURL))
	return env
}

func New(cfg *config.Config) *Fetcher {
	return &Fetcher{
		cfg:  cfg,
		cli:  http.DefaultClient,
		q:    make(chan FetchTask, 200),
		stop: make(chan struct{}),
	}
}

func (f *Fetcher) Enqueue(t FetchTask) {
	f.q <- t
	log.WithField("task", t).Info("task enqueued")
}

func (f *Fetcher) Start() {
	go f.loop()
}

func (f *Fetcher) Stop() {
	f.stop <- struct{}{}
}

func (f *Fetcher) loop() {
	log.Info("fetcher loop started")
	defer log.Info("fetcher loop stopped")
	defer close(f.q)
	defer close(f.stop)

	for {
		select {
		case task := <-f.q:
			f.fetchArtifact(task.RunID, task.ArtifactCfg)
		case <-f.stop:
			return
		}
	}
}

func (f *Fetcher) fetchArtifact(runID int, acfg config.ArtifactConfig) {
	l := log.WithFields(logrus.Fields{
		"run_id":   runID,
		"artifact": acfg.Name,
	})

	// fetch
	l.Info("requesting the list of artifacts for workflow run id")
	al, err := f.listArtifacts(acfg.Repo, runID, acfg.GithubToken)
	if err != nil {
		log.WithError(err).Error("error listing artifacts")
		return
	}

	l.Infof("%d artifacts to download", len(al.Artifacts))
	for _, artf := range al.Artifacts {
		if artf.Expired {
			log.WithField("artifact", artf.Name).Error("artifact is expired")
			continue
		}

		// before fetch
		if len(acfg.Before) != 0 {
			l.Info("runing 'before' commands")
			f.runCommands(acfg.Before, artf.Environ())
			l.Info("finished running 'before' commands")
		} else {
			l.Info("no 'before' commands defined")
		}

		err = f.fetch(artf.ArchiveDownloadURL, acfg.Path, acfg.GithubToken)
		if err != nil {
			log.WithError(err).Error("error fetching artifact")
			return
		}

		// after fetch
		if len(acfg.After) != 0 {
			l.Info("runing 'after' commands")
			f.runCommands(acfg.After, artf.Environ())
			l.Info("finished running 'after' commands")
		} else {
			l.Info("no 'after' commands defined")
		}
	}
}

func (f *Fetcher) runCommands(cmds []string, env []string) {
	l := log.WithField("func", "runCommands")
	for _, c := range cmds {
		cl := l.WithField("cmd", c)
		cl.Info("running command")

		cmd := exec.Command("/bin/bash", "-c", c)
		cmd.Env = append(os.Environ(), env...)

		out, err := cmd.CombinedOutput()
		if err != nil {
			cl.WithField(
				"code", cmd.ProcessState.ExitCode(),
			).WithError(err).Error("error running command")
			break
		}

		cl.WithFields(logrus.Fields{
			"code": cmd.ProcessState.ExitCode(),
			"out":  string(out),
		}).Info("command output")
	}
}

func (f *Fetcher) fetch(url string, path string, token string) error {
	l := log.WithFields(logrus.Fields{
		"func": "fetch",
		"url":  url,
	})

	l.WithField("url", url).Debug("creating request")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "token "+token)

	l.Debug("sending request")
	resp, err := f.cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	l = log.WithFields(logrus.Fields{
		"func": "fetch",
		"path": path,
	})
	l.Debug("creating temp file")
	tfile, err := os.CreateTemp("", "gh-download-")
	if err != nil {
		return err
	}
	l = l.WithField("tmpfile", tfile.Name())
	l.Info("temp file created")

	l.Debug("reading response body to tempfile")
	_, err = io.Copy(tfile, resp.Body)
	if err != nil {
		tfile.Close()
		os.Remove(tfile.Name())
		return err
	}
	tfile.Close()

	defer func() {
		log.WithField("tmpfile", tfile.Name()).Debug("removing temp file")
		os.Remove(tfile.Name())
	}()

	// unzip file to destination dir

	rd, err := zip.OpenReader(tfile.Name())
	if err != nil {
		return err
	}
	defer func() {
		log.WithField("tmpfile", tfile.Name()).Debug("closing temp file")
		rd.Close()
	}()

	l.Info("unzipping artifact")
	for _, zf := range rd.File {
		fpath := filepath.Join(path, zf.Name)

		// if it's an empty directory, create it
		if zf.FileInfo().IsDir() {
			l.Infof("creating directory %s", fpath)
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// otherwise make sure there is a directory for the file
		l.Infof("creating directory %s", filepath.Dir(fpath))
		os.MkdirAll(filepath.Dir(fpath), os.ModePerm)

		// open destination file
		l.Infof("opening file %s", fpath)
		dstFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zf.Mode())
		if err != nil {
			log.WithError(err).WithField("path", fpath).Errorf("error opening dst file")
			continue
		}
		defer func() {
			log.WithField("file", dstFile.Name()).Debug("closing file")
			dstFile.Close()
		}()

		// open source file in zip archive
		zfrd, err := zf.Open()
		if err != nil {
			log.WithError(err).WithField("path", fpath).Errorf("error opening src file")
			continue
		}
		defer zfrd.Close()

		io.Copy(dstFile, zfrd)
		l.Infof("file %s unpacked", fpath)
	}
	return nil
}

func (f *Fetcher) listArtifacts(repo string, runID int, token string) (*GithubArtifactList, error) {
	tokens := strings.Split(repo, "/")
	if len(tokens) != 2 {
		return nil, errors.New("malformed repo string")
	}

	owner, repo := tokens[0], tokens[1]

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs/%d/artifacts", owner, repo, runID)
	log.WithField("url", url).Debug("requesting artifacts list")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "token "+token)

	resp, err := f.cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.WithField("response", string(data)).Debug("got response from github")

	var ghr GithubArtifactList
	err = json.Unmarshal(data, &ghr)
	if err != nil {
		return nil, err
	}

	return &ghr, nil
}
