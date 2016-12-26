package main

import (
	"bufio"
	"bytes"
	"flag"
	"log"
	"math"
	"os"
	"strings"

	"github.com/kr/fs"
	"github.com/robertkrimen/otto/ast"
	"github.com/robertkrimen/otto/parser"
)

var scriptFolder *string

func main() {

	// Collect the location of scripts from command line or default to "./scripts"
	scriptFolder = flag.String("folder", "./samples", "Casper scripts location, defaults to ./scripts")
	flag.Parse()

	// Traverse and process the files in the folder
	testsToRun := traverseFiles(*scriptFolder)

	log.Println("----------------------------------------")
	for _, t := range testsToRun {

		t.RunViaStandardLib()
		// t.RunViaPipe()
	}

}

// loadScripts traverses the files in the specified scriptFolder, and searches
// for the manifest-specific Javascript variables. If all the required variables are found,
// the file is assumed to contain a valid Casper TestSuite, ready to be run.
func traverseFiles(scriptFolder string) []*CasperTest {

	testsToRun := make([]*CasperTest, 0)

	walker := fs.Walk(scriptFolder)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			log.Println("Filesystem walker error: ", err)
			continue
		}

		// Filter out directories and files without a .js extension
		if walker.Stat().IsDir() || !strings.HasSuffix(strings.ToLower(walker.Path()), ".js") {
			continue
		}

		// Analyze the file and add it to the test suites collection
		// if it contains the required info
		testScript, ok := loadScriptFromFile(walker.Path())
		if ok {
			log.Println("Adding valid Casper test: ", testScript.Name)
			testsToRun = append(testsToRun, testScript)
		}
	}

	return testsToRun
}

// loadScriptFromFile reads the contents of a file at the given pathToFile path, and attempts
// to parse the contents as Javascript, and to find a set of agreed-upon manifest variables.
// If those are successfully retrieved, a CasperTest is returned, along with ok set to true.
func loadScriptFromFile(pathToFile string) (*CasperTest, bool) {

	ok := false
	casperTest := &CasperTest{}

	file, err := os.Open(pathToFile)
	if err != nil {
		log.Println("Error opening file at: ", err)
	}
	defer file.Close()

	fileContents := bytes.Buffer{}

	// Superficial and preliminary vetting of the file contents while reading it,
	// to make sure that it contains the designated manifest variables
	var manifestTokenCount = 0
	var outstandingTokenCount = int(math.Pow(2, float64(len(ManifestVariables)))) - 1
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		currentLine := scanner.Bytes()
		currentLine = append(currentLine, '\n') // Add back the newline that scanner.Scan "stole away"

		if manifestTokenCount >= outstandingTokenCount {
			continue
		}

		if _, writeErr := fileContents.Write(currentLine); writeErr != nil {
			log.Println("Error writing file contents: ", writeErr)
			continue
		}

		for i, curManifestVar := range ManifestVariables {
			byteFlagPos := int(math.Pow(2, float64(i)))
			if bytes.Contains(currentLine, []byte(curManifestVar)) &&
				manifestTokenCount&byteFlagPos != byteFlagPos {

				manifestTokenCount = manifestTokenCount + byteFlagPos
				log.Printf("Found %s at index %d (byte flag pos %b), current bitmask map: %d (%b)",
					curManifestVar, i, byteFlagPos, manifestTokenCount, manifestTokenCount)
			}
		}
	}

	if manifestTokenCount < outstandingTokenCount {
		log.Println("Incomplete manifest definition")
		return nil, false
	}

	if err := scanner.Err(); err != nil {
		log.Println("Scanner error: ", err)
	}

	// try to parse the javascript and obtain the values
	program, err := parser.ParseFile(nil, "", fileContents.String(), 0)
	if err != nil {
		log.Println("Error parsing ", pathToFile, ": ", err)
		return nil, false
	}

	for _, declaration := range program.DeclarationList {

		// Only care about variables
		varDecl, ok := declaration.(*ast.VariableDeclaration)
		if ok {

			for _, varExpr := range varDecl.List {

				variableName := varExpr.Name

				for i, curManifestVar := range ManifestVariables {
					if variableName == string(curManifestVar) {

						// Get the value
						variableValue, okVal := varExpr.Initializer.(*ast.StringLiteral)
						if okVal {
							casperTest.SetPropertyByIndex(i, variableValue.Value)
						}
					}
				}
			}
		}
	}

	casperTest.FilePath = pathToFile
	ok = true
	return casperTest, ok
}
