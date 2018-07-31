package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/goji/httpauth"
	"github.com/imroc/req"
	"github.com/jjunk1989/loger"
	"github.com/kardianos/service"
	"github.com/qiniu/log"
)

const appID = "app_8Gji4eEAdDx"

var (
	version string = "master"
	cfg     Configuration
	Log     *loger.Loger
)

type TagInfo struct {
	Version   string `json:"tag_name"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

func githubLatestVersion(repo, name string) (tag TagInfo, err error) {
	githubURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repo, name)
	r := req.New()
	h := req.Header{}
	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken != "" {
		h["Authorization"] = "token " + ghToken
	}
	res, err := r.Get(githubURL, h)
	if err != nil {
		return
	}
	err = res.ToJSON(&tag)
	return
}

func githubUpdate(skipConfirm bool) error {
	repo, name := "soopsio", "gosuv"
	tag, err := githubLatestVersion(repo, name)
	if err != nil {
		fmt.Println("Update failed:", err)
		return err
	}
	if tag.Version == version {
		fmt.Println("No update available, already at the latest version!")
		return nil
	}

	fmt.Println("New version available -- ", tag.Version)
	fmt.Print(tag.Body)

	if !skipConfirm {
		if !askForConfirmation("Would you like to update [Y/n]? ", true) {
			return nil
		}
	}
	fmt.Printf("New version available: %s downloading ... \n", tag.Version)
	// // fetch the update and apply it
	// err = resp.Apply()
	// if err != nil {
	// 	return err
	// }
	cleanVersion := tag.Version
	if strings.HasPrefix(cleanVersion, "v") {
		cleanVersion = cleanVersion[1:]
	}
	osArch := runtime.GOOS + "_" + runtime.GOARCH

	downloadURL := StringFormat("https://github.com/{repo}/{name}/releases/download/{tag}/{name}_{version}_{os_arch}.tar.gz", map[string]interface{}{
		"repo":    "codeskyblue",
		"name":    "gosuv",
		"tag":     tag.Version,
		"version": cleanVersion,
		"os_arch": osArch,
	})
	fmt.Println("Not finished yet. download from:", downloadURL)
	// fmt.Printf("Updated to new version: %s!\n", tag.Version)
	return nil
}

func checkServerStatus() error {
	resp, err := http.Get(cfg.Client.ServerURL + "/api/status")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var ret JSONResponse
	err = json.Unmarshal(body, &ret)
	if err != nil {
		return errors.New("json loads error: " + string(body))
	}
	if ret.Status != 0 {
		return fmt.Errorf("%v", ret.Value)
	}
	return nil
}

type ServiceManager struct {
}

func (s *ServiceManager) Start(service.Service) (err error) {
	if !service.Interactive() {
		Log.Info("under sys service")
	}
	// Start should not block. Do the actual work async.
	go s.run()
	return
}

func (s *ServiceManager) Stop(service.Service) (err error) {
	return
}

func (s *ServiceManager) run() {
	Log.Info("startServerDirect")
	suv, hdlr, err := newSupervisorHandler()
	if err != nil {
		Log.Error(err)
		return
	}
	if err = newDistributed(suv, hdlr); err != nil {
		Log.Error(err)
		return
	}

	cfg, _ = readConf(path.Join(defaultGosuvDir, "config.yml"))
	auth := cfg.Server.HttpAuth
	if auth.Enabled {
		hdlr = httpauth.SimpleBasicAuth(auth.User, auth.Password)(hdlr)
	}
	http.Handle("/", hdlr)
	addr := cfg.Server.Addr
	suv.AutoStartPrograms()
	Log.Info("server listen on:", addr)
	if err = http.ListenAndServe(addr, nil); err != nil {
		Log.Error("http listen error", err)
	}
	Log.Info("server stoped")
}

func init() {
	defaultGosuvDir = os.Getenv("GOSUV_HOME_DIR")
	if defaultGosuvDir == "" {
		log.Info("cant find env: GOSUV_HOME_DIR, please set.")
		defaultGosuvDir = filepath.Join(UserHomeDir(), ".gosuv")
	}
	log.Info("defaultGosuvDir:", defaultGosuvDir)
	http.Handle("/res/", http.StripPrefix("/res/", http.FileServer(Assets))) // http.StripPrefix("/res/", Assets))
	Log = loger.NewLoger(path.Join(defaultGosuvDir, "servermanagerLog"))
}

func main() {
	//	var defaultConfigPath = filepath.Join(defaultGosuvDir, "conf/config.yml")
	svcFlag := flag.String("service", "", "start, stop, restart, install, uninstall.")
	flag.Parse()

	svcConfig := &service.Config{
		Name:        "ServerManager",
		DisplayName: "Mananger server",
		Description: "Mananger server.",
	}

	s, err := service.New(&ServiceManager{}, svcConfig)
	if err != nil {
		Log.Error(err)
	}

	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			Log.Printf("Valid actions: %q\n", service.ControlAction)
			Log.Error(err)
		}
		return
	}
	err = s.Run()
	if err != nil {
		Log.Error(err)
	}
}
