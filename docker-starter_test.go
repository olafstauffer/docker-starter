package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type mock_environment struct {
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	env    *[]string
}

func (e mock_environment) getStdout() io.Writer {
	return e.stdout
}
func (e mock_environment) getStderr() io.Writer {
	return e.stderr
}
func (e mock_environment) getEnvVariables() []string {
	return *e.env
}

// ShouldContainOutput receives one buffer and one or more strings to look for..
func ShouldContainOutput(actual interface{}, expected ...interface{}) string {

	output := actual.(bytes.Buffer)

	for _, e := range expected {

		want := e.(string)

		if !strings.Contains(output.String(), want) {
			return fmt.Sprintf("missing %s from output", want)
		}
	}
	return ""
}

func ShouldNotContainOutput(actual interface{}, expected ...interface{}) string {

	output := actual.(bytes.Buffer)
	if output.Len() != 0 {
		return fmt.Sprintf("unexpected output: '%s'", output.String())
	}
	return ""
}

type Msg struct {
	Message  string
	Expected string
	Actual   string
}

func getStructuredError(message string, expected interface{}, actual interface{}) string {

	expectedString := fmt.Sprintf("%+v", expected)
	actualString := fmt.Sprintf("%+v", actual)
	m := Msg{message, expectedString, actualString}
	serialized, _ := json.Marshal(m)
	return string(serialized)
}

func ShouldHaveLength(actual interface{}, expected ...interface{}) string {

	actualValue := reflect.ValueOf(actual)
	actualLength := actualValue.Len()
	expectedLength := expected[0].(int)

	if actualLength != expectedLength {
		msg := fmt.Sprintf("wrong length for %s, %+v", reflect.TypeOf(actual), actual)
		return getStructuredError(msg, expectedLength, actualLength)
	}
	return ""
}

func TestFuncAddNew(t *testing.T) {

	Convey("Given a empty map", t, func() {

		Convey("The function should add the key/value pair to the map", func() {

			vars := make(map[string][]string)

			result := addNew(&vars, "key", "value")

			So(result, ShouldBeTrue)
			So(vars, ShouldHaveLength, 1)
			So(vars["key"], ShouldHaveLength, 1)
			So(vars["key"][0], ShouldEqual, "value")

		})
	})

	Convey("Given a filled map", t, func() {

		Convey("The function should add new key value pairs", func() {

			vars := make(map[string][]string)
			vars["key"] = append(vars["key"], "value1")

			result := addNew(&vars, "key", "value2")

			So(result, ShouldBeTrue)
			So(vars, ShouldHaveLength, 1)
			So(vars["key"], ShouldHaveLength, 2)
			So(vars["key"][0], ShouldEqual, "value1")
			So(vars["key"][1], ShouldEqual, "value2")
		})

		Convey("The function should not add already set key value pairs", func() {

			vars := make(map[string][]string)
			vars["key"] = append(vars["key"], "value1")

			result := addNew(&vars, "key", "value1")

			So(result, ShouldBeFalse)
			So(vars, ShouldHaveLength, 1)
			So(vars["key"], ShouldHaveLength, 1)
			So(vars["key"][0], ShouldEqual, "value1")
		})
	})

}

func TestFuncReadExtendedVariables(t *testing.T) {

	Convey("Given environment variables without a link variable", t, func() {
		Convey("The function should return a map containing the environment", func() {

			var stdout, stderr bytes.Buffer
			env := []string{"FOO=BAR"}
			e := mock_environment{&stdout, &stderr, &env}

			result := readExtendedVariables(e)

			Convey("The resulting arrays should be of correct length", func() {
				So(result, ShouldHaveLength, 1)
			})

			Convey("The result should contain the correct key", func() {
				So(result["FOO"], ShouldNotBeNil)
				So(result["FOO"], ShouldHaveLength, 1)
				So(result["FOO"][0], ShouldEqual, "BAR")
			})

			Convey("The output should contain expected strings", func() {
				So(stdout, ShouldNotContainOutput)
				So(stderr, ShouldNotContainOutput)
			})
		})

	})

	Convey("Given a link environment variable", t, func() {
		Convey("The function should add additional keys to the result", func() {

			var stdout, stderr bytes.Buffer
			env := []string{"APP_PORT_1234_TCP=tcp://hostname:1234"}
			e := mock_environment{&stdout, &stderr, &env}

			result := readExtendedVariables(e)

			Convey("The result should be of correct length", func() {
				So(len(result), ShouldEqual, 3)
			})

			Convey("The result should contain a key with the application url", func() {
				So(result["APP_URL"], ShouldNotBeNil)
				So(result["APP_URL"], ShouldHaveLength, 1)
				So(result["APP_URL"][0], ShouldEqual, "http://hostname:1234")
			})

			Convey("The result should contain a key with the application+port url", func() {
				So(result["APP_1234_URL"], ShouldNotBeNil)
				So(result["APP_1234_URL"], ShouldHaveLength, 1)
				So(result["APP_1234_URL"][0], ShouldEqual, "http://hostname:1234")
			})

			Convey("The output should contain expected strings", func() {
				So(stderr, ShouldContainOutput, "use:", "APP_URL", "APP_1234_URL")
				So(stdout, ShouldNotContainOutput)
			})
		})

		Convey("The function should not overwrite existing variables", func() {

			var stdout, stderr bytes.Buffer
			env := []string{
				"APP_URL=FOO",
				"APP_1234_URL=BAR",
				"APP_PORT_1234_TCP=tcp://hostname:1234",
			}
			e := mock_environment{&stdout, &stderr, &env}

			result := readExtendedVariables(e)

			Convey("The result should consist of three elements", func() {
				So(len(result), ShouldEqual, 3)
			})

			Convey("The result should contain the existing app variable", func() {
				So(result["APP_URL"], ShouldNotBeNil)
				So(result["APP_URL"], ShouldHaveLength, 2)
				So(result["APP_URL"][0], ShouldEqual, "FOO")
			})

			Convey("The result should contain the existing app+port variable", func() {
				So(result["APP_1234_URL"], ShouldNotBeNil)
				So(result["APP_1234_URL"], ShouldHaveLength, 2)
				So(result["APP_1234_URL"][0], ShouldEqual, "BAR")
			})

			Convey("The output should contain expected strings", func() {
				So(stderr, ShouldContainOutput, "use:", "APP_URL", "APP_1234_URL")
				So(stdout, ShouldNotContainOutput)
			})

		})

		Convey("The function should not generate keys from invalid link variables", func() {

			var stdout, stderr bytes.Buffer
			env := []string{"KIBANA_PORT_5601_TCP=tcp://INVALID"}
			e := mock_environment{&stdout, &stderr, &env}

			result := readExtendedVariables(e)

			So(result, ShouldHaveLength, 1)
			So(result["KIBANA_URL"], ShouldBeEmpty)
			So(stderr, ShouldContainOutput, "found invalid link value")
			So(stdout, ShouldNotContainOutput)
		})

	})

	Convey("Given multiple link environment variables", t, func() {

		Convey("With multiple ports for one application", func() {

			Convey("The function should add multiple additional keys to the result", func() {

				var stdout, stderr bytes.Buffer
				env := []string{
					"ES_PORT_9200_TCP=tcp://172.17.0.63:9200",
					"ES_PORT_9300_TCP=tcp://172.17.0.63:9300",
				}
				e := mock_environment{&stdout, &stderr, &env}

				result := readExtendedVariables(e)

				Convey("The result should give the correct number of keys", func() {
					So(result, ShouldHaveLength, 5)
				})

				Convey("The application url key should be set correctly", func() {
					So(result["ES_URL"], ShouldNotBeNil)
					So(result["ES_URL"], ShouldHaveLength, 2)
					So(result["ES_URL"][0], ShouldEqual, "http://172.17.0.63:9200")
					So(result["ES_URL"][1], ShouldEqual, "http://172.17.0.63:9300")
				})

				Convey("The application+port url should be set correctly", func() {
					So(result["ES_9200_URL"], ShouldNotBeNil)
					So(result["ES_9200_URL"], ShouldHaveLength, 1)
					So(result["ES_9200_URL"][0], ShouldEqual, "http://172.17.0.63:9200")
					So(result["ES_9300_URL"], ShouldNotBeNil)
					So(result["ES_9300_URL"], ShouldHaveLength, 1)
					So(result["ES_9300_URL"][0], ShouldEqual, "http://172.17.0.63:9300")
				})

				Convey("The output should be as expected", func() {
					So(stderr, ShouldContainOutput, "use:", "ES_URL", "ES_9200_URL", "ES_9300_URL")
					So(stdout, ShouldNotContainOutput)
				})
			})
		})

		Convey("With multiple application (same port)", func() {

			Convey("The function should add multiple additional keys to the result", func() {

				var stdout, stderr bytes.Buffer
				env := []string{
					"APP_1_PORT_1234_TCP=tcp://hostname1:1234",
					"APP_2_PORT_1234_TCP=tcp://hostname2:1234",
				}
				e := mock_environment{&stdout, &stderr, &env}

				result := readExtendedVariables(e)

				Convey("The result should give the correct number of keys", func() {
					So(len(result), ShouldEqual, 4)
				})

				Convey("The application url key should be set correctly", func() {
					So(result["APP_URL"], ShouldNotBeNil)
					So(result["APP_URL"], ShouldHaveLength, 2)
					So(result["APP_URL"][0], ShouldEqual, "http://hostname1:1234")
					So(result["APP_URL"][1], ShouldEqual, "http://hostname2:1234")
				})

				Convey("The application+port url should be set correctly", func() {
					So(result["APP_1234_URL"], ShouldNotBeNil)
					So(result["APP_1234_URL"], ShouldHaveLength, 2)
					So(result["APP_1234_URL"][0], ShouldEqual, "http://hostname1:1234")
					So(result["APP_1234_URL"][1], ShouldEqual, "http://hostname2:1234")
				})

				Convey("The output should be as expected", func() {
					So(stderr, ShouldContainOutput, "use:", "APP_URL", "APP_1234_URL")
					So(stdout, ShouldNotContainOutput)
				})
			})
		})

		Convey("With multiple application with multiple ports", func() {

			Convey("The function should add multiple additional keys to the result", func() {

				var stdout, stderr bytes.Buffer
				env := []string{
					"APP_1_PORT_1000_TCP=tcp://hostname1:1000",
					"APP_1_PORT_2000_TCP=tcp://hostname1:2000",
					"APP_2_PORT_1000_TCP=tcp://hostname2:1000",
					"APP_2_PORT_2000_TCP=tcp://hostname2:2000",
				}
				e := mock_environment{&stdout, &stderr, &env}

				result := readExtendedVariables(e)

				Convey("The result should give the correct number of keys", func() {
					So(len(result), ShouldEqual, 7)
				})

				Convey("The application url key should be set correctly", func() {
					So(result["APP_URL"], ShouldNotBeNil)
					So(result["APP_URL"], ShouldHaveLength, 4)
					So(result["APP_URL"][0], ShouldEqual, "http://hostname1:1000")
					So(result["APP_URL"][1], ShouldEqual, "http://hostname1:2000")
					So(result["APP_URL"][2], ShouldEqual, "http://hostname2:1000")
					So(result["APP_URL"][3], ShouldEqual, "http://hostname2:2000")
				})

				Convey("The application+port url should be set correctly", func() {
					So(result["APP_1000_URL"], ShouldNotBeNil)
					So(result["APP_1000_URL"], ShouldHaveLength, 2)
					So(result["APP_1000_URL"][0], ShouldEqual, "http://hostname1:1000")
					So(result["APP_1000_URL"][1], ShouldEqual, "http://hostname2:1000")
					So(result["APP_2000_URL"], ShouldNotBeNil)
					So(result["APP_2000_URL"], ShouldHaveLength, 2)
					So(result["APP_2000_URL"][0], ShouldEqual, "http://hostname1:2000")
					So(result["APP_2000_URL"][1], ShouldEqual, "http://hostname2:2000")
				})

				Convey("The output should be as expected", func() {
					So(stderr, ShouldContainOutput, "use:", "APP_URL", "APP_1000_URL", "APP_2000_URL")
					So(stdout, ShouldNotContainOutput)
				})
			})
		})

	})

}

func TestFuncFillArgs(t *testing.T) {

	Convey("Given parameters without template markup", t, func() {

		Convey("The function should return the input", func() {
			var stdout, stderr bytes.Buffer
			env := []string{}
			e := mock_environment{&stdout, &stderr, &env}

			var cmdSrc string = "command"
			var dirSrc string = "dir"
			vars := make(map[string][]string)
			vars["FOO"] = append(vars["FOO"], "BAR")

			cmdResult, dirResult, err := fillArgs(e, cmdSrc, dirSrc, vars)

			So(err, ShouldBeNil)
			So(cmdResult, ShouldEqual, "command")
			So(dirResult, ShouldEqual, "dir")
			So(stdout, ShouldNotContainOutput)
			So(stderr, ShouldNotContainOutput)
		})
	})

	Convey("Given parameters with valid template markup", t, func() {

		Convey("The function should fill the markup", func() {
			var stdout, stderr bytes.Buffer
			env := []string{}
			e := mock_environment{&stdout, &stderr, &env}

			var cmdSrc string = "command_{{E .FOO}}_{{E .FOO}}"
			var dirSrc string = "dir_{{E .FOO}}"
			vars := make(map[string][]string)
			vars["FOO"] = append(vars["FOO"], "BAR")

			cmdResult, dirResult, err := fillArgs(e, cmdSrc, dirSrc, vars)

			So(err, ShouldBeNil)
			So(cmdResult, ShouldEqual, "command_BAR_BAR")
			So(dirResult, ShouldEqual, "dir_BAR")
			So(stdout, ShouldNotContainOutput)
			So(stderr, ShouldNotContainOutput)
		})
	})

	Convey("Given parameters with invalid markup in 'cmd'", t, func() {

		Convey("The function should respond with an error", func() {
			var stdout, stderr bytes.Buffer
			env := []string{}
			e := mock_environment{&stdout, &stderr, &env}

			var cmdSrc string = "command{{.FOO"
			var dirSrc string = ""
			vars := make(map[string][]string)
			vars["FOO"] = append(vars["FOO"], "BAR")

			cmdResult, dirResult, err := fillArgs(e, cmdSrc, dirSrc, vars)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unclosed action")
			So(cmdResult, ShouldBeEmpty)
			So(dirResult, ShouldBeEmpty)
			So(stderr, ShouldContainOutput, "error processing")
			So(stdout, ShouldNotContainOutput)
		})
	})

	Convey("Given parameters with invalid markup in 'dir'", t, func() {

		Convey("The function should respond with an error", func() {
			var stdout, stderr bytes.Buffer
			env := []string{}
			e := mock_environment{&stdout, &stderr, &env}

			var cmdSrc string = ""
			var dirSrc string = "dir{{.FOO"
			vars := make(map[string][]string)
			vars["FOO"] = append(vars["FOO"], "BAR")

			cmdResult, dirResult, err := fillArgs(e, cmdSrc, dirSrc, vars)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unclosed action")
			So(cmdResult, ShouldBeEmpty)
			So(dirResult, ShouldBeEmpty)
			So(stderr, ShouldContainOutput, "error processing")
			So(stdout, ShouldNotContainOutput)
		})
	})

	Convey("Given parameters with markup and empty environment", t, func() {

		Convey("The function should respond with an error", func() {
			var stdout, stderr bytes.Buffer
			env := []string{}
			e := mock_environment{&stdout, &stderr, &env}

			var cmdSrc string = "command_{{.FOO}}"
			var dirSrc string = ""
			vars := make(map[string][]string)

			cmdResult, dirResult, err := fillArgs(e, cmdSrc, dirSrc, vars)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "could not fill all markup")
			So(cmdResult, ShouldBeEmpty)
			So(dirResult, ShouldBeEmpty)
			So(stderr, ShouldContainOutput, "error processing cmd")
			So(stdout, ShouldNotContainOutput)
		})
	})
}

// func TestFuncExtract(t *testing.T) {

// 	Convey("Given a empty value", t, func() {

// 		Convey("The function should return an error", func() {

// 			var template string = "{{E }}"
// 			vars := make(map[string][]string)

// 			result, err := processString(template, vars)

// 			So(err, ShouldNotBeNil)
// 			So(err.Error(), ShouldContainSubstring, "wrong number of args")
// 			So(result, ShouldBeEmpty)
// 		})
// 	})

// 	Convey("Given a single value", t, func() {

// 		Convey("The function should return the value as string", func() {

// 			var template string = "{{E .FOO}}"
// 			vars := make(map[string][]string)
// 			vars["FOO"] = append(vars["FOO"], "BAR")

// 			result, err := processString(template, vars)

// 			So(err, ShouldBeNil)
// 			So(result, ShouldEqual, "BAR")
// 		})
// 	})

// 	Convey("Given multiple value", t, func() {

// 		Convey("And no seperator given", func() {

// 			Convey("The function should return a the values joined by ','", func() {

// 				var template string = "{{E .FOO}}"
// 				vars := make(map[string][]string)
// 				vars["FOO"] = append(vars["FOO"], "value1")
// 				vars["FOO"] = append(vars["FOO"], "value2")

// 				result, err := processString(template, vars)

// 				So(err, ShouldBeNil)
// 				So(result, ShouldEqual, "value1,value2")
// 			})
// 		})

// 		Convey("With a given seperator", func() {

// 			Convey("The function should return a the values joined by stat separator", func() {

// 				var template string = "{{E .FOO \"#\"}}"
// 				vars := make(map[string][]string)
// 				vars["FOO"] = append(vars["FOO"], "value1")
// 				vars["FOO"] = append(vars["FOO"], "value2")

// 				result, err := processString(template, vars)

// 				So(err, ShouldBeNil)
// 				So(result, ShouldEqual, "value1#value2")
// 			})
// 		})
// 	})
// }

func TestFuncProcessString(t *testing.T) {

	Convey("Given a valid template", t, func() {

		Convey("The function should return the input", func() {

			var template string = "TEST"
			vars := make(map[string][]string)
			vars["FOO"] = append(vars["FOO"], "BAR")

			result, err := processString(template, vars)

			So(err, ShouldBeNil)
			So(result, ShouldEqual, "TEST")
		})

		Convey("Given a template with function E", func() {

			Convey("Given the key exists and vars has multiple elements", func() {

				Convey("The function should return the first element", func() {

					var template string = "{{E .FOO}}"

					vars1 := make(map[string][]string)
					vars1["FOO"] = append(vars1["FOO"], "BAR", "BAR2")
					result, err := processString(template, vars1)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "BAR")

					vars2 := make(map[string][]string)
					vars2["FOO"] = append(vars2["FOO"], "test", "test2")
					result, err = processString(template, vars2)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "test")
				})
			})

			Convey("Given the key does not exist", func() {

				Convey("The function should return an empty string", func() {

					var template string = "{{E .FOO}}"

					vars := make(map[string][]string)
					result, err := processString(template, vars)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "")

				})
			})

		})

		Convey("Given a template with function J (without separator)", func() {

			Convey("Given the vars hold multiple elements", func() {

				Convey("The function should return the elements joined by ','", func() {

					var template string = "{{J .FOO}}"

					vars1 := make(map[string][]string)
					vars1["FOO"] = append(vars1["FOO"], "BAR", "BAR2")
					result, err := processString(template, vars1)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "BAR,BAR2")

					vars2 := make(map[string][]string)
					vars2["FOO"] = append(vars2["FOO"], "test", "test2")
					result, err = processString(template, vars2)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "test,test2")
				})
			})
			Convey("Given the vars hold one element", func() {

				Convey("The function should return the element", func() {

					var template string = "{{J .FOO}}"

					vars1 := make(map[string][]string)
					vars1["FOO"] = append(vars1["FOO"], "BAR")
					result, err := processString(template, vars1)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "BAR")

					vars2 := make(map[string][]string)
					vars2["FOO"] = append(vars2["FOO"], "test")
					result, err = processString(template, vars2)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "test")
				})
			})
			Convey("Given the vars hold no element", func() {

				Convey("The function should return an empty string", func() {

					var template string = "{{J .FOO}}"

					vars := make(map[string][]string)
					vars["FOO"] = []string{}
					result, err := processString(template, vars)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "")
				})
			})
			Convey("Given the key does not exist", func() {

				Convey("The function should return an empty string", func() {

					var template string = "{{J .FOO}}"

					vars := make(map[string][]string)
					result, err := processString(template, vars)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "")
				})
			})
		})

		Convey("Given a template with function J (with separator)", func() {

			Convey("Given the vars hold multiple elements", func() {

				Convey("The function should return the joined elements", func() {

					var template string = "{{J .FOO \"#\"}}"

					vars1 := make(map[string][]string)
					vars1["FOO"] = append(vars1["FOO"], "BAR", "BAR2")
					result, err := processString(template, vars1)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "BAR#BAR2")

					vars2 := make(map[string][]string)
					vars2["FOO"] = append(vars2["FOO"], "test", "test2")
					result, err = processString(template, vars2)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "test#test2")
				})
			})

			Convey("Given the vars hold one element", func() {

				Convey("The function should return the element", func() {

					var template string = "{{J .FOO \"#\"}}"

					vars1 := make(map[string][]string)
					vars1["FOO"] = append(vars1["FOO"], "BAR")
					result, err := processString(template, vars1)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "BAR")

					vars2 := make(map[string][]string)
					vars2["FOO"] = append(vars2["FOO"], "test")
					result, err = processString(template, vars2)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "test")
				})
			})

			Convey("Given the vars hold no element", func() {

				Convey("The function should return an empty string", func() {

					var template string = "{{J .FOO \"#\"}}"

					vars := make(map[string][]string)
					vars["FOO"] = []string{}
					result, err := processString(template, vars)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "")
				})
			})

			Convey("Given key doen not exit in vars", func() {

				Convey("The function should return an empty string", func() {

					var template string = "{{J .FOO \"#\"}}"

					vars := make(map[string][]string)
					result, err := processString(template, vars)
					So(err, ShouldBeNil)
					So(result, ShouldEqual, "")
				})
			})
		})

	})

	Convey("Given a invalid template", t, func() {

		Convey("The function should return an error", func() {

			var template string = "{{"
			vars := make(map[string][]string)
			vars["FOO"] = append(vars["FOO"], "BAR")

			result, err := processString(template, vars)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unexpected")
			So(result, ShouldEqual, "")
		})
	})

	Convey("Given a template with a join", t, func() {

		Convey("The function should return the joined keys", func() {

			var template string = "{{J .FOO \"#\"}}"
			vars := make(map[string][]string)
			vars["FOO"] = append(vars["FOO"], "e1", "e2", "e3")

			result, err := processString(template, vars)

			So(err, ShouldBeNil)
			So(result, ShouldEqual, "e1#e2#e3")
		})
	})
}

func TestFuncFindTemplateFiles(t *testing.T) {

	Convey("Given a existing directory", t, func() {

		Convey("Without template files", func() {

			Convey("The function should return a empty list", func() {
				var stdout, stderr bytes.Buffer
				env := []string{}
				e := mock_environment{&stdout, &stderr, &env}

				dirname, _ := ioutil.TempDir("", "_docker-starter")
				defer os.RemoveAll(dirname)

				files, err := findTemplateFiles(e, dirname)

				So(err, ShouldBeNil)
				So(len(files), ShouldEqual, 0)
				So(stdout, ShouldNotContainOutput)
				So(stderr, ShouldNotContainOutput)
			})
		})

		Convey("With template files", func() {

			Convey("The function should return a list of the template files", func() {
				var stdout, stderr bytes.Buffer
				env := []string{}
				e := mock_environment{&stdout, &stderr, &env}

				dirname, _ := ioutil.TempDir("", "_docker-starter")
				defer os.RemoveAll(dirname)

				createFile(dirname, "no_template.txt", "TEST")
				createFile(dirname, "template1.txt.tmpl", "TEST")
				createFile(dirname, "template2.txt.tmpl", "TEST")

				files, err := findTemplateFiles(e, dirname)

				So(err, ShouldBeNil)
				So(len(files), ShouldEqual, 2)
				So(files, ShouldContain, "template1.txt.tmpl")
				So(stderr, ShouldContainOutput, "found template")
				So(stdout, ShouldNotContainOutput)
			})
		})
	})

	Convey("Given a invalid directory", t, func() {

		Convey("The function should return a error", func() {
			var stdout, stderr bytes.Buffer
			env := []string{}
			e := mock_environment{&stdout, &stderr, &env}

			dirname, _ := ioutil.TempDir("", "_docker-starter")
			defer os.RemoveAll(dirname)

			invalidname := path.Join(dirname, "invalid")
			files, err := findTemplateFiles(e, invalidname)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no such file or directory")
			So(len(files), ShouldEqual, 0)
			So(stderr, ShouldContainOutput, "cannot read dir")
			So(stdout, ShouldNotContainOutput)
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

				var stdout, stderr bytes.Buffer
				env := []string{}
				e := mock_environment{&stdout, &stderr, &env}

				vars := make(map[string][]string)
				vars["FOO"] = append(vars["FOO"], "BAR")

				dirname, _ := ioutil.TempDir("", "_docker-starter")
				defer os.RemoveAll(dirname)

				targetname := fmt.Sprintf("test-%d", rand.Intn(10000))
				templatename := fmt.Sprintf("%s.tmpl", targetname)
				createFile(dirname, templatename, "{{E .FOO}}")

				err := processTemplate(e, dirname, templatename, vars, true)

				contents, _ := readFile(dirname, targetname)

				So(err, ShouldBeNil)
				So(len(readDir(dirname)), ShouldEqual, 2)
				So(readDir(dirname), ShouldContain, targetname)
				So(contents, ShouldEqual, "BAR")
				So(stdout.String(), ShouldBeEmpty)
				So(stderr.String(), ShouldBeEmpty)
			})
		})

		Convey("And a target file that already exists", func() {

			Convey("And no force flag given", func() {

				Convey("The function should return an error and not write to file", func() {

					var stdout, stderr bytes.Buffer
					env := []string{}
					e := mock_environment{&stdout, &stderr, &env}

					vars := make(map[string][]string)
					vars["FOO"] = append(vars["FOO"], "BAR")

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
					So(stderr.String(), ShouldNotBeEmpty)
					So(stdout.String(), ShouldBeEmpty)
				})
			})

			Convey("With force flag given", func() {

				Convey("And a writable file", func() {

					Convey("The function should log a warning an write to file", func() {

						var stdout, stderr bytes.Buffer
						env := []string{}
						e := mock_environment{&stdout, &stderr, &env}

						vars := make(map[string][]string)
						vars["FOO"] = append(vars["FOO"], "BAR")

						dirname, _ := ioutil.TempDir("", "_docker-starter")
						defer os.RemoveAll(dirname)

						targetname := "test.txt"
						createFile(dirname, targetname, "DONT OVERWRITE")
						templatename := fmt.Sprintf("%s.tmpl", targetname)
						createFile(dirname, templatename, "{{E .FOO}}")

						err := processTemplate(e, dirname, templatename, vars, true)

						contents, _ := readFile(dirname, targetname)

						So(err, ShouldBeNil)
						So(len(readDir(dirname)), ShouldEqual, 2)
						So(readDir(dirname), ShouldContain, targetname)
						So(contents, ShouldEqual, "BAR")
						So(stderr, ShouldContainOutput, "overwriting existing file")
						So(stdout, ShouldNotContainOutput)
					})
				})

				Convey("And a readonly file", func() {

					Convey("The function should return an error", func() {

						var stdout, stderr bytes.Buffer
						env := []string{}
						e := mock_environment{&stdout, &stderr, &env}

						vars := make(map[string][]string)
						vars["FOO"] = append(vars["FOO"], "BAR")

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
						So(stderr, ShouldContainOutput, "error creating file")
						So(stdout, ShouldNotContainOutput)
					})
				})

			})

		})

	})

	Convey("Given a invalid filename to a template", t, func() {

		Convey("The function should return an error", func() {

			var stdout, stderr bytes.Buffer
			env := []string{}
			e := mock_environment{&stdout, &stderr, &env}

			vars := make(map[string][]string)
			vars["FOO"] = append(vars["FOO"], "BAR")

			dirname, _ := ioutil.TempDir("", "_docker-starter")
			defer os.RemoveAll(dirname)

			invalid_templatename := "test"
			err := processTemplate(e, dirname, invalid_templatename, vars, true)

			So(err, ShouldNotBeNil)
			So(len(readDir(dirname)), ShouldEqual, 0)
			So(stderr, ShouldContainOutput, "error processing template")
			So(stdout, ShouldNotContainOutput)
		})

	})

	Convey("Given a invalid template file", t, func() {

		Convey("The function should return an error and not write", func() {

			var stdout, stderr bytes.Buffer
			env := []string{}
			e := mock_environment{&stdout, &stderr, &env}

			vars := make(map[string][]string)
			vars["FOO"] = append(vars["FOO"], "BAR")

			dirname, _ := ioutil.TempDir("", "_docker-starter")
			defer os.RemoveAll(dirname)

			templatename := "test.txt.tmpl"
			createFile(dirname, templatename, "{{.FOO")

			err := processTemplate(e, dirname, templatename, vars, true)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unclosed action")
			So(len(readDir(dirname)), ShouldEqual, 1)
			So(stderr, ShouldContainOutput, "error processing template")
			So(stdout, ShouldNotBeEmpty)
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

			var stdout, stderr bytes.Buffer
			env := []string{}
			e := mock_environment{&stdout, &stderr, &env}

			args := []string{}
			vars := map[string][]string{}

			err := executeCommand(e, "invalid-command-76238429", args, vars)

			So(err, ShouldNotBeNil)
			So(stderr, ShouldContainOutput, "error executing command")
			So(stdout, ShouldNotContainOutput)

		})

	})
	Convey("Given a valid command", t, func() {

		Convey("The command should run", func() {

			var stdout, stderr bytes.Buffer
			env := []string{}
			e := mock_environment{&stdout, &stderr, &env}

			cmd := "echo"
			args := []string{"HELLO"}
			vars := map[string][]string{}

			err := executeCommand(e, cmd, args, vars)

			So(err, ShouldBeNil)
			So(stderr, ShouldContainOutput, "process", "started")
			So(stdout, ShouldContainOutput, "HELLO")

		})

		Convey("The command should see the given environment variables", func() {

			var stdout, stderr bytes.Buffer
			env := []string{}
			e := mock_environment{&stdout, &stderr, &env}

			cmd := "env"
			args := []string{}
			random := fmt.Sprintf("foo-%d", rand.Intn(10000))
			vars := map[string][]string{}
			vars["FOO"] = append(vars["FOO"], "BAR", "BAR2")
			vars[random] = append(vars[random], "rand1", "rand2")

			err := executeCommand(e, cmd, args, vars)
			// fmt.Printf("OUT: %+v, ERR: %+v\n", stdout.String(), stderr.String())

			So(err, ShouldBeNil)
			So(stderr, ShouldContainOutput, "process", "started")
			So(stdout, ShouldContainOutput, "FOO", "BAR", random, "rand1")
		})

	})

}
