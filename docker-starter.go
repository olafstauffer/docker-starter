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
		elasticsearch: "{{.ELASTICSEARCHCONTAINER_HTTP}}"
		port: {{.KIBANA_PORT}}






For more info on go templates see: http://golang.org/pkg/text/template/

*/

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"text/template"
)

// create interface to help testing with log output and environment variables
type DockerStarterEnvironment interface {
	getEnvVariables() []string
	getLogger() *log.Logger
}

type environment struct {
}

func (environment) getEnvVariables() []string {
	return os.Environ()
}
func (environment) getLogger() *log.Logger {
	return log.New(os.Stdout, "docker-starter: ", 0)
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

	execErr := executeCommand(e, cmd, os.Args, vars)
	exitOnError(execErr)

	os.Exit(0)
}

func exitOnError(err error) {
	if err != nil {
		os.Exit(1)
	}
}

func readExtendedVariables(env DockerStarterEnvironment) (result map[string]string) {

	logger := env.getLogger()
	result = make(map[string]string)

	// link environment variables should look like this
	var re_linkkey = regexp.MustCompile(`^([^_]+)_.*_TCP$`)
	var re_linkvalue = regexp.MustCompile(`^(.*)://(.*):(.*)$`)

	// copy environment vars to result and extend link variables
	for _, e := range env.getEnvVariables() {

		// copy original variable to result
		pair := strings.Split(e, "=")
		key, value := pair[0], pair[1] // use "key", "value" for better readability
		result[key] = value
		// logger.Printf("key=%s, value=%s", key, value)

		// look for link variables
		key_elements := re_linkkey.FindStringSubmatch(key)
		if key_elements == nil {
			continue
		}

		logger.Printf("found link variable %s=%s", key, value)

		value_elements := re_linkvalue.FindStringSubmatch(value)
		if len(value_elements) < 3 {
			logger.Printf("found invalid link value: %+v", value)
			continue
		}

		new_key := fmt.Sprintf("%s_URL", key_elements[1])
		if _, ok := result[new_key]; ok { // don't overwrite existing variables
			continue
		}

		new_value := fmt.Sprintf("http://%s:%s", value_elements[2], value_elements[3])
		result[new_key] = new_value
		logger.Printf("creating new variable %s=%s\n", new_key, new_value)
	}

	return
}

func fillArgs(env DockerStarterEnvironment, cmdSrc string, dirSrc string, vars map[string]string) (cmd string, dir string, err error) {

	logger := env.getLogger()

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

func processString(src string, vars map[string]string) (string, error) {

	t, err := template.New("Template").Parse(src)
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

	logger := env.getLogger()

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

func processTemplate(env DockerStarterEnvironment, dirname string, filename string, vars map[string]string, force bool) (err error) {

	logger := env.getLogger()

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
	t, err := template.ParseFiles(filename)
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

func executeCommand(env DockerStarterEnvironment, cmd string, osArgs []string, vars map[string]string) error {

	logger := env.getLogger()

	// find the given command
	binary, err := exec.LookPath(cmd)
	if err != nil {
		logger.Printf("error executing command: %s", err)
		return err
	}
	logger.Printf("starting: %s", binary)

	// prepend the args list with the command
	args := []string{cmd}
	for _, a := range osArgs {
		args = append(args, a)
	}

	// transform the map back to a list of type "key=value"
	var osVariables []string
	for key, value := range vars {
		osVariables = append(osVariables, fmt.Sprintf("%s=%s", key, value))
	}

	// // call the command
	err = syscall.Exec(binary, args, osVariables)
	if err != nil {
		return err
	}

	return nil
}
