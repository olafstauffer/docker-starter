/*
Copyright 2014 Olaf Stauffer

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
docker-starter - docker start command to replace template markup with environment vars

Motivation:

Some applications do not allow to specify the connection configuration they need via
the command arguments and therefore cannot be configured via the docker cmd arguments.
Usually those application need some config files on startup.

A typical pattern is to provide those config files via docker volume to the application.
Unfortunatelly this can become very messy setting up complex dev environments. Especially
when a connection string is the only information that needs to be modified and we want to
use docker links (or the link within a fig.yml). This usually results in a lot of local
config files that have to be manually synced with a fig.yml.


How does docker-starter work:

   * meant to be used as docker cmd - runs on container start
   * needs config templates (go template syntax) within the image
   * replaces tempate markup with current environment variables
   * writes config files
   * starts the actual container application



Example: Creating Kibana image with docker-starter

	Dockerfile






Example: Environment with Elasticsearch and Kibana

	Kibana is running on port 8000 and connecting to the linked elasticsearchcontainer.


	fig.yml
		kibana:
  			image: ollo/kibana
  			ports:
  			- "8000:8000"
  			links:
  			- elasticsearch
  			environment:
  			- KIBANA_PORT=8000
		elasticsearchcontainer:
		  image: dockerfile/elasticsearch
		  ports:
		  - "9200:9200"
		  - "9300:9300"

	.../kibana/config/kibana.yml.tmpl (in image)
		...
		elasticsearch: "{{.ELASTICSEARCHCONTAINER_URL}}"
		port: {{join .KIBANA_PORT ","}}



Notes:

    * For more info on go templates see: http://golang.org/pkg/text/template/
    * Static binaries built with :goxc -d=binaries -pv=0.2.0 -bc="linux,386 darwin"

*/

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

// create interface to help testing with log output and environment variables
type DockerStarterEnvironment interface {
	getStdout() io.Writer
	getStderr() io.Writer
	getEnvVariables() []string
}

type environment struct {
}

func (environment) getStdout() io.Writer {
	return os.Stdout
}
func (environment) getStderr() io.Writer {
	return os.Stderr
}
func (environment) getEnvVariables() []string {
	return os.Environ()
}

func main() {

	rawCmd := flag.String("cmd", "", "command to execute")
	rawDir := flag.String("dir", "", "directory to read templates (*.tmpl) and write output to")
	force := flag.Bool("force", false, "overwrite existing files")
	flag.Parse()

	e := environment{}

	// read environment and extend link variables
	vars := readExtendedVariables(e)

	cmd, dir, argErr := fillArgs(e, *rawCmd, *rawDir, vars)
	exitOnError(argErr)

	files, findErr := findTemplateFiles(e, dir)
	exitOnError(findErr)

	for _, file := range files {
		err := processTemplate(e, dir, file, vars, *force)
		exitOnError(err)
	}

	execErr := executeCommand(e, cmd, flag.Args(), vars)
	exitOnError(execErr)

	os.Exit(0)
}

func exitOnError(err error) {
	if err != nil {
		os.Exit(1)
	}
}

func getLogger(env DockerStarterEnvironment) *log.Logger {
	return log.New(env.getStderr(), "docker-starter: ", log.LstdFlags)
}

func readExtendedVariables(env DockerStarterEnvironment) (result map[string][]string) {

	logger := getLogger(env)
	result = make(map[string][]string)

	summary := make(map[string]bool)

	// convert slice of strings from environment to resulting data structure
	// here every key can have multiple value associated with it
	// note: the template function "E"  expects thos values to be ordered, the most
	// important one at the first position
	for _, e := range env.getEnvVariables() {
		pair := strings.Split(e, "=")
		result[pair[0]] = append(result[pair[0]], pair[1])
	}

	// make sore we process the keys in a deterministic order
	keys := []string{}
	for k, _ := range result {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// now  iterate over every key value pair, search for link keys
	// and add additional keys generated from the link values
	for _, key := range keys {

		// look for link variables
		app, _, appport := parseLinkkey(key)
		if app == "" {
			continue
		}

		// expect link variables to have a certain structure
		_, host, port, err := parseLinkvalue(result[key][0])
		if err != nil {
			logger.Println(err)
			continue
		}
		// logger.Printf("found link variable %s -> host=%s, port=%s",
		// 	key, host, port)

		urlValue := fmt.Sprintf("http://%s:%s", host, port)

		// create app url key
		appKey := fmt.Sprintf("%s_URL", app)
		if isSet := addNew(&result, appKey, urlValue); isSet {
			summary[appKey] = true
			// logger.Printf("created new variable %s=%s\n", appKey, urlValue)
		}

		// create app + port url key
		appPortKey := fmt.Sprintf("%s_%s_URL", app, appport)
		if isSet := addNew(&result, appPortKey, urlValue); isSet {
			summary[appPortKey] = true
			// logger.Printf("created new variable %s=%s\n", appPortKey, urlValue)
		}
	}

	for key, _ := range summary {
		logger.Printf("use: %s = %+v", key, result[key])
	}

	return
}

func addNew(m *map[string][]string, key string, value string) bool {

	found := false
	for _, old := range (*m)[key] {
		if old == value {
			found = true
			break
		}
	}

	if found {
		return false
	}

	(*m)[key] = append((*m)[key], value)

	return true
}

func parseLinkkey(key string) (app string, idx string, port string) {
	var re = regexp.MustCompile(`^([^_]+)_(\d*).*PORT_(\d+)_TCP$`)

	k := re.FindStringSubmatch(key)
	if k == nil {
		return
	}

	app = k[1]
	idx = k[2]
	port = k[3]
	return
}

func parseLinkvalue(value string) (schema string, host string, port string, err error) {
	var re = regexp.MustCompile(`^(.*)://(.*):(\d+)$`)

	v := re.FindStringSubmatch(value)
	if len(v) < 3 {
		err = fmt.Errorf("found invalid link value in %s", value)
		return
	}

	schema = v[1]
	host = v[2]
	port = v[3]

	return
}

func fillArgs(env DockerStarterEnvironment, cmdSrc string, dirSrc string, vars map[string][]string) (cmd string, dir string, err error) {

	logger := getLogger(env)

	cmd, err = processString(cmdSrc, vars)
	if err != nil {
		logger.Printf("error processing cmd: %s (%s)", cmdSrc, err)
		return
	}

	dir, err = processString(dirSrc, vars)
	if err != nil {
		logger.Printf("error processing dir: %s (%s)", dirSrc, err)
		return
	}

	return
}

var funcMap template.FuncMap = template.FuncMap{
	"E": extractFirstElement,
	"J": extractJoinedElements,
}

func extractFirstElement(values []string) string {
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

func extractJoinedElements(values []string, sepArg ...string) string {

	var sep string = ","
	if len(sepArg) > 0 {
		sep = sepArg[0]
	}

	return strings.Join(values, sep)
}

func processString(src string, vars map[string][]string) (string, error) {

	t, err := template.New("Template").Funcs(funcMap).Parse(src)
	if err != nil {
		return "", err
	}

	var buffer bytes.Buffer
	err = t.Execute(&buffer, vars)
	if err != nil {
		return "", err
	}

	// currently (go 1.4) there is no proper way to check if all fields
	// in a template have been replaced (https://github.com/golang/go/issues/6288)
	//
	// workaround which interprets the default "<no value>" as error
	if strings.Contains(buffer.String(), "<no value>") {
		return "", fmt.Errorf("could not fill all markup in: %s", src)
	}

	return buffer.String(), nil
}

func findTemplateFiles(env DockerStarterEnvironment, root string) (result []string, err error) {

	logger := getLogger(env)

	var files []os.FileInfo
	files, err = ioutil.ReadDir(root)
	if err != nil {
		logger.Printf("cannot read dir: %s", err)
		return
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tmpl") {
			logger.Printf("found template: %s", file.Name())
			result = append(result, file.Name())
		}
	}
	return
}

func processTemplate(env DockerStarterEnvironment, dirname string, filename string, vars map[string][]string, force bool) (err error) {

	logger := getLogger(env)

	os.Chdir(dirname)

	suffixStart := strings.LastIndex(filename, ".tmpl")
	if suffixStart < 0 {
		err = fmt.Errorf("error processing template: invalid template name", filename)
		logger.Println(err)
		return err
	}

	targetname := filename[:suffixStart]

	// don't overwrite a file without the flag
	_, fileExistsErr := os.Stat(targetname)
	if !os.IsNotExist(fileExistsErr) {
		if !force {
			err := fmt.Errorf("error processing template: destinaton exists: %s", targetname)
			logger.Println(err)
			return err
		} else {
			logger.Printf("overwriting existing file: %s", targetname)
		}
	}

	// find tempate files (src files)
	t, err := template.New(filename).Funcs(funcMap).ParseFiles(filename)
	if err != nil {
		logger.Printf("error processing template: %s", err)
		return err
	}

	w, err := os.Create(targetname)
	if err != nil {
		logger.Printf("error creating file: %s", err)
		return err
	}
	defer w.Close()

	err = t.Execute(w, vars)
	if err != nil {
		return err
	}

	return
}

func executeCommand(env DockerStarterEnvironment, cmd string, args []string, vars map[string][]string) error {

	logger := getLogger(env)

	// transform the map back to a list of type "key=value"
	var commandVars []string
	for k, v := range vars {
		commandVars = append(commandVars, fmt.Sprintf("%s=%s", k, v[0]))
	}

	command := exec.Command(cmd, args...)
	command.Stdout = env.getStdout()
	command.Stderr = env.getStderr()
	command.Env = commandVars

	err := command.Start()
	if err != nil {
		logger.Printf("error executing command: %s", err)
		return err
	}
	logger.Printf("process %d started", command.Process.Pid)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs) // catch all signals
	go func() {
		for sig := range sigs { // keep receiving signals
			command.Process.Signal(sig) // forward signal to command
		}
	}()
	err = command.Wait() // block until command exits

	return nil
}
