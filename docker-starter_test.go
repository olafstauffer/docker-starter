package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type mock_environment struct {
	env []string
	log *bytes.Buffer
}

func (e mock_environment) getEnvVariables() []string {
	return e.env
}
func (e mock_environment) getLogger() *log.Logger {
	return log.New(e.log, "docker-starter: ", log.LstdFlags|log.Lmicroseconds)
}

func TestFuncReadExtendedVariables(t *testing.T) {

	Convey("Given environment variables without a link variable", t, func() {
		Convey("The function should return a map containing the environment", func() {

			log := bytes.Buffer{}
			env := []string{"FOO=BAR"}
			e := mock_environment{env, &log}

			result := readExtendedVariables(e)

			So(result["FOO"], ShouldNotBeNil)
			So(result["FOO"], ShouldEqual, "BAR")
			So(len(result), ShouldEqual, 1)
			So(log.String(), ShouldBeEmpty)
		})

	})

	Convey("Given a link environment variable", t, func() {
		Convey("The function should add an additional key to the result", func() {

			log := bytes.Buffer{}
			env := []string{"KIBANA_PORT_5601_TCP=tcp://127.0.0.1:5601"}
			e := mock_environment{env, &log}

			result := readExtendedVariables(e)

			So(result["KIBANA_URL"], ShouldNotBeNil)
			So(result["KIBANA_URL"], ShouldEqual, "http://127.0.0.1:5601")
			So(len(result), ShouldEqual, 2)
			So(log.String(), ShouldContainSubstring, "found link variable")
			So(log.String(), ShouldContainSubstring, "creating new variable")
		})

		Convey("The function should never overwrite an existing variable", func() {

			Convey("With the existing variable at the start of then list", func() {
				log := bytes.Buffer{}
				env := []string{"KIBANA_URL=FOO", "KIBANA_PORT_5601_TCP=tcp://127.0.0.1:5601"}
				e := mock_environment{env, &log}

				result := readExtendedVariables(e)

				So(result["KIBANA_URL"], ShouldNotBeNil)
				So(result["KIBANA_URL"], ShouldEqual, "FOO")
				So(len(result), ShouldEqual, 2)
				So(log.String(), ShouldContainSubstring, "found link variable")
				So(log.String(), ShouldNotContainSubstring, "creating new variable")
			})

			Convey("With the existing variable at the end of then list", func() {

				log := bytes.Buffer{}
				env := []string{"KIBANA_PORT_5601_TCP=tcp://127.0.0.1:5601", "KIBANA_URL=FOO"}
				e := mock_environment{env, &log}

				result := readExtendedVariables(e)

				So(result["KIBANA_URL"], ShouldNotBeNil)
				So(result["KIBANA_URL"], ShouldEqual, "FOO")
				So(len(result), ShouldEqual, 2)
				So(log.String(), ShouldContainSubstring, "found link variable")
				// note: no check for log line containing "creating new variable" here
				// beause its ok to create the variable as long as it gets overwritten later
				// So(log.String(), ShouldNotContainSubstring, "creating new variable")
			})

		})

		Convey("The function should ignore invalid link variables", func() {

			log := bytes.Buffer{}
			env := []string{"KIBANA_PORT_5601_TCP=tcp://INVALID"}
			e := mock_environment{env, &log}

			result := readExtendedVariables(e)

			So(result["KIBANA_URL"], ShouldBeEmpty)
			So(len(result), ShouldEqual, 1)
			So(log.String(), ShouldContainSubstring, "found link variable")
			So(log.String(), ShouldContainSubstring, "found invalid link value")
		})

	})
}

func TestFuncFillArgs(t *testing.T) {

	Convey("Given parameters without template markup", t, func() {

		Convey("The function should return the input", func() {
			log := bytes.Buffer{}
			env := []string{}
			e := mock_environment{env, &log}

			var cmdSrc string = "command"
			var dirSrc string = "dir"
			vars := map[string]string{"FOO": "BAR"}

			cmdResult, dirResult, err := fillArgs(e, cmdSrc, dirSrc, vars)

			So(err, ShouldBeNil)
			So(cmdResult, ShouldEqual, "command")
			So(dirResult, ShouldEqual, "dir")
			So(log.String(), ShouldBeEmpty)
		})
	})

	Convey("Given parameters with valid template markup", t, func() {

		Convey("The function should fill the markup", func() {
			log := bytes.Buffer{}
			env := []string{}
			e := mock_environment{env, &log}

			var cmdSrc string = "command_{{.FOO}}_{{.FOO}}"
			var dirSrc string = "dir_{{.FOO}}"
			vars := map[string]string{"FOO": "BAR"}

			cmdResult, dirResult, err := fillArgs(e, cmdSrc, dirSrc, vars)

			So(err, ShouldBeNil)
			So(cmdResult, ShouldEqual, "command_BAR_BAR")
			So(dirResult, ShouldEqual, "dir_BAR")
			So(log.String(), ShouldBeEmpty)
		})
	})

	Convey("Given parameters with invalid markup in 'cmd'", t, func() {

		Convey("The function should respond with an error", func() {
			log := bytes.Buffer{}
			env := []string{}
			e := mock_environment{env, &log}

			var cmdSrc string = "command{{.FOO"
			var dirSrc string = ""
			vars := map[string]string{"FOO": "BAR"}

			cmdResult, dirResult, err := fillArgs(e, cmdSrc, dirSrc, vars)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unclosed action")
			So(cmdResult, ShouldBeEmpty)
			So(dirResult, ShouldBeEmpty)
			So(log.String(), ShouldContainSubstring, "error processing")
		})
	})

	Convey("Given parameters with invalid markup in 'dir'", t, func() {

		Convey("The function should respond with an error", func() {
			log := bytes.Buffer{}
			env := []string{}
			e := mock_environment{env, &log}

			var cmdSrc string = ""
			var dirSrc string = "dir{{.FOO"
			vars := map[string]string{"FOO": "BAR"}

			cmdResult, dirResult, err := fillArgs(e, cmdSrc, dirSrc, vars)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unclosed action")
			So(cmdResult, ShouldBeEmpty)
			So(dirResult, ShouldBeEmpty)
			So(log.String(), ShouldContainSubstring, "error processing")
		})
	})

	Convey("Given parameters with markup and empty environment", t, func() {

		Convey("The function should respond with an error", func() {
			log := bytes.Buffer{}
			env := []string{}
			e := mock_environment{env, &log}

			var cmdSrc string = "command_{{.FOO}}"
			var dirSrc string = ""
			vars := map[string]string{}

			cmdResult, dirResult, err := fillArgs(e, cmdSrc, dirSrc, vars)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "could not fill all markup")
			So(cmdResult, ShouldBeEmpty)
			So(dirResult, ShouldBeEmpty)
			So(log.String(), ShouldContainSubstring, "error processing cmd")
		})
	})
}

func TestFuncFindTemplateFiles(t *testing.T) {

	Convey("Given a existing directory", t, func() {

		Convey("Without template files", func() {

			Convey("The function should return a empty list", func() {
				log := bytes.Buffer{}
				env := []string{}
				e := mock_environment{env, &log}

				dirname, _ := ioutil.TempDir("", "_docker-starter")
				defer os.RemoveAll(dirname)

				files, err := findTemplateFiles(e, dirname)

				So(err, ShouldBeNil)
				So(len(files), ShouldEqual, 0)
				So(log.String(), ShouldBeEmpty)
			})
		})

		Convey("With template files", func() {

			Convey("The function should return a list of the template files", func() {
				log := bytes.Buffer{}
				env := []string{}
				e := mock_environment{env, &log}

				dirname, _ := ioutil.TempDir("", "_docker-starter")
				defer os.RemoveAll(dirname)

				createFile(dirname, "no_template.txt", "TEST")
				createFile(dirname, "template1.txt.tmpl", "TEST")
				createFile(dirname, "template2.txt.tmpl", "TEST")

				files, err := findTemplateFiles(e, dirname)

				So(err, ShouldBeNil)
				So(len(files), ShouldEqual, 2)
				So(files, ShouldContain, "template1.txt.tmpl")
				So(log.String(), ShouldContainSubstring, "found template")
			})
		})
	})

	Convey("Given a invalid directory", t, func() {

		Convey("The function should return a error", func() {
			log := bytes.Buffer{}
			env := []string{}
			e := mock_environment{env, &log}

			dirname, _ := ioutil.TempDir("", "_docker-starter")
			defer os.RemoveAll(dirname)

			invalidname := path.Join(dirname, "invalid")
			files, err := findTemplateFiles(e, invalidname)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no such file or directory")
			So(len(files), ShouldEqual, 0)
			So(log.String(), ShouldNotBeEmpty)
		})
	})

}

func createFile(dir string, name string, text string, mode ...os.FileMode) string {

	var filemode os.FileMode
	if len(mode) > 0 {
		filemode = mode[0]
	} else {
		filemode = 0644
	}

	if name == "" {
		file, _ := ioutil.TempFile(dir, "")
		defer file.Close()
		file.WriteString(text)
		name = file.Name()
	} else {
		fullname := path.Join(dir, name)
		ioutil.WriteFile(fullname, []byte(text), filemode)
	}
	return name
}

func TestFuncProcessTemplate(t *testing.T) {

	Convey("Given a valid template file", t, func() {

		Convey("Given a empty target directory", func() {

			Convey("The function should create a processed file", func() {

				log := bytes.Buffer{}
				env := []string{}
				e := mock_environment{env, &log}

				vars := map[string]string{"FOO": "BAR"}
				dirname, _ := ioutil.TempDir("", "_docker-starter")
				defer os.RemoveAll(dirname)

				targetname := fmt.Sprintf("test-%d", rand.Intn(10000))
				templatename := fmt.Sprintf("%s.tmpl", targetname)
				createFile(dirname, templatename, "{{.FOO}}")

				err := processTemplate(e, dirname, templatename, vars, true)

				contents, _ := readFile(dirname, targetname)

				So(err, ShouldBeNil)
				So(len(readDir(dirname)), ShouldEqual, 2)
				So(readDir(dirname), ShouldContain, targetname)
				So(contents, ShouldEqual, "BAR")
				So(log.String(), ShouldBeEmpty)
			})
		})

		Convey("And a target file that already exists", func() {

			Convey("And no force flag given", func() {

				Convey("The function should return an error and not write to file", func() {

					log := bytes.Buffer{}
					env := []string{}
					e := mock_environment{env, &log}

					vars := map[string]string{"FOO": "BAR"}
					dirname, _ := ioutil.TempDir("", "_docker-starter")
					defer os.RemoveAll(dirname)

					targetname := "test.txt"
					createFile(dirname, targetname, "DONT OVERWRITE")
					templatename := fmt.Sprintf("%s.tmpl", targetname)
					createFile(dirname, templatename, "{{.FOO}}")

					err := processTemplate(e, dirname, templatename, vars, false)

					contents, _ := readFile(dirname, targetname)

					So(err, ShouldNotBeNil)
					So(len(readDir(dirname)), ShouldEqual, 2)
					So(readDir(dirname), ShouldContain, targetname)
					So(contents, ShouldEqual, "DONT OVERWRITE")
					So(log.String(), ShouldNotBeEmpty)
				})
			})

			Convey("With force flag given", func() {

				Convey("And a writable file", func() {

					Convey("The function should log a warning an write to file", func() {

						log := bytes.Buffer{}
						env := []string{}
						e := mock_environment{env, &log}

						vars := map[string]string{"FOO": "BAR"}
						dirname, _ := ioutil.TempDir("", "_docker-starter")
						defer os.RemoveAll(dirname)

						targetname := "test.txt"
						createFile(dirname, targetname, "DONT OVERWRITE")
						templatename := fmt.Sprintf("%s.tmpl", targetname)
						createFile(dirname, templatename, "{{.FOO}}")

						err := processTemplate(e, dirname, templatename, vars, true)

						contents, _ := readFile(dirname, targetname)

						So(err, ShouldBeNil)
						So(len(readDir(dirname)), ShouldEqual, 2)
						So(readDir(dirname), ShouldContain, targetname)
						So(contents, ShouldEqual, "BAR")
						So(log.String(), ShouldNotBeEmpty)
						So(log.String(), ShouldContainSubstring, "overwriting existing file")
					})
				})

				Convey("And a readonly file", func() {

					Convey("The function should return an error", func() {

						log := bytes.Buffer{}
						env := []string{}
						e := mock_environment{env, &log}

						vars := map[string]string{"FOO": "BAR"}
						dirname, _ := ioutil.TempDir("", "_docker-starter")
						defer os.RemoveAll(dirname)

						targetname := "test.txt"
						createFile(dirname, targetname, "DONT OVERWRITE", 0444)
						templatename := fmt.Sprintf("%s.tmpl", targetname)
						createFile(dirname, templatename, "{{.FOO}}")

						err := processTemplate(e, dirname, templatename, vars, true)

						contents, _ := readFile(dirname, targetname)

						So(err, ShouldNotBeNil)
						So(err.Error(), ShouldContainSubstring, "permission denied")
						So(len(readDir(dirname)), ShouldEqual, 2)
						So(readDir(dirname), ShouldContain, targetname)
						So(contents, ShouldEqual, "DONT OVERWRITE")
						So(log.String(), ShouldNotBeEmpty)
						So(log.String(), ShouldContainSubstring, "error creating file")
					})
				})

			})

		})

	})

	Convey("Given a invalid filename to a template", t, func() {

		Convey("The function should return an error", func() {

			log := bytes.Buffer{}
			env := []string{}
			e := mock_environment{env, &log}

			vars := map[string]string{"FOO": "BAR"}
			dirname, _ := ioutil.TempDir("", "_docker-starter")
			defer os.RemoveAll(dirname)

			invalid_templatename := "test"
			err := processTemplate(e, dirname, invalid_templatename, vars, true)

			So(err, ShouldNotBeNil)
			So(len(readDir(dirname)), ShouldEqual, 0)
			So(log.String(), ShouldNotBeEmpty)
			So(log.String(), ShouldContainSubstring, "error processing template")
		})

	})

	Convey("Given a invalid template file", t, func() {

		Convey("The function should return an error and not write", func() {

			log := bytes.Buffer{}
			env := []string{}
			e := mock_environment{env, &log}

			vars := map[string]string{"FOO": "BAR"}
			dirname, _ := ioutil.TempDir("", "_docker-starter")
			defer os.RemoveAll(dirname)

			templatename := "test.txt.tmpl"
			createFile(dirname, templatename, "{{.FOO")

			err := processTemplate(e, dirname, templatename, vars, true)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unclosed action")
			So(len(readDir(dirname)), ShouldEqual, 1)
			So(log.String(), ShouldNotBeEmpty)
			So(log.String(), ShouldContainSubstring, "error processing template")
		})
	})

}

func readDir(dir string) (files []string) {
	fileinfos, _ := ioutil.ReadDir(dir)
	for _, f := range fileinfos {
		files = append(files, f.Name())
	}
	return
}
func readFile(dir string, name string) (string, error) {
	fullname := path.Join(dir, name)
	data, err := ioutil.ReadFile(fullname)
	return string(data), err
}

func TestFuncExecuteCommand(t *testing.T) {

	Convey("Given a invalid command", t, func() {

		Convey("The function should return an error", func() {

			log := bytes.Buffer{}
			env := []string{}
			e := mock_environment{env, &log}

			args := []string{}
			vars := map[string]string{}

			err := executeCommand(e, "invalid-command-76238429", args, vars)

			So(err, ShouldNotBeNil)
			So(log.String(), ShouldNotBeEmpty)

		})

	})
	Convey("Given a valid command", t, func() {

		Convey("The command should be found", nil)

		Convey("The command should be started", nil)

		Convey("The command should see the given environment variables", nil)

		Convey("The command should see the given command line options", nil)

	})

}