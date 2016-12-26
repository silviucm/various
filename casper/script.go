package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os/exec"

	"gopkg.in/pipe.v2"
)

// Variable names that must be present in the CasperJS scripts in order to get parsed
var ManifestVariables = [...]string{"MANIFEST_SCRIPT_ID", "MANIFEST_SCRIPT_NAME",
	"MANIFEST_SCRIPT_DESC"}

// CasperTest holds essential information about a CasperJS test script
type CasperTest struct {
	Id          string
	FilePath    string
	Name        string
	Description string
}

// SetPropertyByIndex determines which of the fields to set for the CasperTest instance,
// based on the  manifestVarIndex and the ManifestVariables array
func (c *CasperTest) SetPropertyByIndex(manifestVarIndex int, value string) {

	switch manifestVarIndex {
	case 0:
		c.Id = value
	case 1:
		c.Name = value
	case 2:
		c.Description = value
	}

}

// RunViaStandardLib launches CasperJS in test mode, using the Go standard library
// functionality. The input file for Casper is provided by c.FilePath.
func (c *CasperTest) RunViaStandardLib() {

	log.Println("RunViaStandardLib - About to run test: ", c.Name)
	casperCmd := exec.Command("casperjs", "test", c.FilePath)
	stdOut, err := casperCmd.StdoutPipe()
	if err != nil {
		log.Printf("Run() - Test %s - casperCmd.StdoutPipe() Error: %s", c.Name, err.Error())
		return
	}

	err = casperCmd.Start()
	if err != nil {
		log.Printf("Run() - Test %s - casperCmd.Start() Error: %s", c.Name, err.Error())
		return
	}

	defer stdOut.Close()

	for {

		r := bufio.NewReader(stdOut)
		line, _, err := r.ReadLine()

		if err != nil {

			// End-of-file (EOF) are treated as errors in Go io operations, so we need
			// to make the distinction
			if err == io.EOF || err == io.ErrClosedPipe || err == io.ErrUnexpectedEOF {
				log.Printf("Run() - Test %s - Casper Output Ended", c.Name)
				break
			} else {

				// deal with the regular errors
				log.Printf("Run() - Test %s - Error reading casper output at line: %s",
					c.Name, err.Error())
				return
			}
		}

		fmt.Printf("CasperJS: %s\n", line)
	}

	// wait for the command to cleanly execute
	err = casperCmd.Wait()
	if err != nil {
		log.Printf("Run() - Test %s - casperCmd.Wait() Error: %s", c.Name, err.Error())
		return
	}
}

// RunViaPipe launches CasperJS in test mode, using the the pipe package
// The input file for Casper is provided by c.FilePath.
func (c *CasperTest) RunViaPipe() {

	log.Println("RunViaPipe - About to run test: ", c.Name)
	cPipe := pipe.Line(
		pipe.Exec("casperjs", "test", c.FilePath),
		pipe.Filter(func(line []byte) bool {
			fmt.Printf("CasperJS: %s\n", line)
			return true
		}),
	)

	err := pipe.Run(cPipe)
	if err != nil {
		log.Printf("Run() - Test %s - pipe.Run() Error: %s", c.Name, err.Error())
	}

}
