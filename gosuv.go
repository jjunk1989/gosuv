package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	_ "os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
	// default program name
	defaultProgramPath string = "programs.yml"
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
	Server *http.Server
	Suv    *Supervisor
}

func NewServiceManager() *ServiceManager {
	return &ServiceManager{
		Server: &http.Server{},
	}
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
	if !service.Interactive() {
		Log.Info("under sys service")
		s.Suv.KillAll()
	}
	Log.Info("ServiceManager stoped.")
	return s.shutdown()
}

func (s *ServiceManager) run() {
	Log.Info("start serviceManager.")
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

	mux := http.NewServeMux()
	mux.Handle("/", hdlr)
	Log.Info("Assets path: ", Assets, http.Dir(path.Join(defaultGosuvDir, "res")))
	mux.Handle("/res/", http.StripPrefix("/res/", http.FileServer(Assets)))

	mux.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
		Log.Info("receive test")
		w.Write([]byte("test"))
	})
	s.Server.Addr = cfg.Server.Addr
	s.Server.Handler = mux

	suv.AutoStartPrograms()
	// save suv
	s.Suv = suv
	Log.Info("server listen on:", s.Server.Addr)

	if err = s.Server.ListenAndServe(); err != nil {
		Log.Error("http listen error", err)
	}

	Log.Info("server stoped")
}

func (s *ServiceManager) killAll() {
	s.Suv.KillAll()
}

func (s *ServiceManager) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// close all program
	Log.Info("shut down all program")
	err := s.Server.Shutdown(ctx)
	if err != nil {
		Log.Error("stop proxy server error:", err)
	}
	return err
}

func init() {
	//	defaultGosuvDir = os.Getenv("GOSUV_HOME_DIR")
	//	if defaultGosuvDir == "" {
	//		//		defaultGosuvDir = filepath.Join(UserHomeDir(), ".gosuv")
	//		exePath := filepath.Dir(os.Args[0])
	//		defaultGosuvDir, _ = filepath.Abs(exePath)
	//		log.Info("未找到系统环境变量: GOSUV_HOME_DIR.")
	//		log.Info("使用当前目录:", defaultGosuvDir)
	//	}
	exePath := filepath.Dir(os.Args[0])
	defaultGosuvDir, _ = filepath.Abs(exePath)
	log.Info("使用当前目录:", defaultGosuvDir)
	//	http.Handle("/res/", http.StripPrefix("/res/", http.FileServer(Assets))) // http.StripPrefix("/res/", Assets))
	Log = loger.NewLoger(path.Join(defaultGosuvDir, "servermanagerLog"))
	// init asset folder
	//	log.Info("asstes path:", defaultGosuvDir, path.Join(defaultGosuvDir, "res"))
	Assets = http.Dir(path.Join(defaultGosuvDir, "res"))
}

func main() {
	//	var defaultConfigPath = filepath.Join(defaultGosuvDir, "conf/config.yml")
	pathFlag := flag.String("program", "", "program config file name. use: .\\gosuv.exe -program ttt.yml")
	svcFlag := flag.String("service", "", "start, stop, restart, install, uninstall.")
	flag.Parse()

	if *pathFlag != "" {
		// config program name use flag
		//  .\gosuv.exe -program ttt.yml
		log.Info("use program config file:", *pathFlag)
		defaultProgramPath = *pathFlag
	}

	svcConfig := &service.Config{
		Name:        "ServerManager",
		DisplayName: "Mananger server",
		Description: "Mananger server.",
	}

	s, err := service.New(NewServiceManager(), svcConfig)
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
