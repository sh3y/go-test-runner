package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/TwiN/go-color"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var (
	version = "1.0.0"
)

func main() {
	rootCmd, _ := initRootCommand()
	if err := rootCmd.Execute(); err != nil {
		var gracefulStop = make(chan os.Signal)
		signal.Notify(gracefulStop, syscall.SIGTERM)
		signal.Notify(gracefulStop, syscall.SIGINT)
		go func() {
			sig := <-gracefulStop
			fmt.Printf(color.InYellow("warning:")+" caught sig: %+v", sig)
			fmt.Println(color.InGreen("info:") + " Wait for 2 second to finish processing")
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}()
	}
}

func initRootCommand() (*cobra.Command, *cmdFlags) {
	flags := &cmdFlags{}
	rootCmd := &cobra.Command{
		Use:  "test-runner",
		Long: "Runs Go Tests and Stores the Results in a JSON File.",
		RunE: func(cmd *cobra.Command, args []string) (e error) {
			startTime := time.Now()
			if err := checkIfStdinIsPiped(); err != nil {
				return err
			}
			stdin := os.Stdin
			stdinScanner := bufio.NewScanner(stdin)
			defer func() {
				_ = stdin.Close()
			}()
			startTestTime := time.Now()
			allPackageNames, allTests, err := readTestDataFromStdIn(stdinScanner, flags, cmd)
			if err != nil {
				return errors.New(err.Error() + "\n")
			}
			elapsedTestTime := time.Since(startTestTime)
			var testFileDetailByPackage testFileDetailsByPackage
			if flags.listFlag != "" {
				testFileDetailByPackage, err = getAllDetails(flags.listFlag)
			} else {
				testFileDetailByPackage, err = getPackageDetails(allPackageNames)
			}
			if err != nil {
				return err
			}
			CWD, err := os.Getwd()
			if err != nil {
				return err
			}
			discoveryFile := filepath.Join(CWD, flags.outputFlag)
			execFile := filepath.Join(CWD, flags.execFlag)
			dfile, err := Exists(discoveryFile)

			if err != nil {
				return err
			}
			if dfile {
				fmt.Println(color.InYellow("warning: ") + "discovery file already exists. Please specify a new file. ")
				return errors.New("Discovery File already exists")
			}
			efile, err := Exists(execFile)

			if err != nil {
				return err
			}
			if efile {
				fmt.Println(color.InYellow("warning: ") + "execution file already exists. Specify a new file to continue. ")
				return errors.New("Execution File already exists")
			}
			err = generateReport(allTests, testFileDetailByPackage, elapsedTestTime, discoveryFile, execFile)
			if err != nil {
				return err
			}
			elapsedTime := time.Since(startTime)
			elapsedTimeMsg := []byte(fmt.Sprintf(color.Ize(color.Cyan, "[test-runner]")+" finished in %s\n", elapsedTime))
			if _, err := cmd.OutOrStdout().Write(elapsedTimeMsg); err != nil {
				return err
			}
			return nil
		},
	}
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Prints the version of test-runner",
		RunE: func(cmd *cobra.Command, args []string) error {
			msg := fmt.Sprintf("test-runner: v%s", version)
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), msg); err != nil {
				return err
			}
			return nil
		},
	}
	rootCmd.AddCommand(versionCmd)
	rootCmd.PersistentFlags().StringVarP(&flags.titleFlag,
		"title",
		"t",
		"test-runner",
		"the title text shown in the test report")
	rootCmd.PersistentFlags().StringVarP(&flags.sizeFlag,
		"size",
		"s",
		"24",
		"the size (in pixels) of the clickable indicator for test result groups")
	rootCmd.PersistentFlags().IntVarP(&flags.groupSize,
		"groupSize",
		"g",
		20,
		"the number of tests per test group indicator")
	rootCmd.PersistentFlags().StringVarP(&flags.execFlag,
		"exec",
		"e",
		"exec.report.json",
		"the execution report file")
	rootCmd.PersistentFlags().StringVarP(&flags.outputFlag,
		"discovery",
		"d",
		"discovery.report.json",
		"the JSON Report file.")
	rootCmd.PersistentFlags().BoolVarP(&flags.verbose,
		"verbose",
		"v",
		false,
		"Verbose processing output ")
	return rootCmd, flags
}

func readTestDataFromStdIn(stdinScanner *bufio.Scanner, flags *cmdFlags, cmd *cobra.Command) (allPackageNames map[string]*types.Nil, allTests map[string]*testStatus, e error) {
	allTests = map[string]*testStatus{}
	allPackageNames = map[string]*types.Nil{}

	// read from stdin and parse "go test" results
	for stdinScanner.Scan() {
		lineInput := stdinScanner.Bytes()
		if flags.verbose {
			newline := []byte("\n")
			if _, err := cmd.OutOrStdout().Write(append(lineInput, newline[0])); err != nil {
				return nil, nil, err
			}
		}
		goTestOutputRow := &goTestOutputRow{}
		if err := json.Unmarshal(lineInput, goTestOutputRow); err != nil {
			return nil, nil, err
		}
		if goTestOutputRow.TestName != "" {
			var status *testStatus
			key := goTestOutputRow.Package + "." + goTestOutputRow.TestName
			if _, exists := allTests[key]; !exists {
				status = &testStatus{
					TestName: goTestOutputRow.TestName,
					Package:  goTestOutputRow.Package,
					Output:   []string{},
				}
				allTests[key] = status
			} else {
				status = allTests[key]
			}
			if goTestOutputRow.Action == "pass" || goTestOutputRow.Action == "fail" || goTestOutputRow.Action == "skip" {
				if goTestOutputRow.Action == "pass" {
					status.Passed = true
				}
				if goTestOutputRow.Action == "skip" {
					status.Skipped = true
				}
				status.ElapsedTime = goTestOutputRow.Elapsed
			}
			allPackageNames[goTestOutputRow.Package] = nil
			if strings.Contains(goTestOutputRow.Output, "--- PASS:") {
				goTestOutputRow.Output = strings.TrimSpace(goTestOutputRow.Output)
			}
			status.Output = append(status.Output, goTestOutputRow.Output)
		}
	}
	return allPackageNames, allTests, nil
}

func getAllDetails(listFile string) (testFileDetailsByPackage, error) {
	testFileDetailByPackage := testFileDetailsByPackage{}
	f, err := os.Open(listFile)
	defer f.Close()
	if err != nil {
		return nil, err
	}
	list := json.NewDecoder(f)
	for list.More() {
		goListJSON := goListJSON{}
		if err := list.Decode(&goListJSON); err != nil {
			return nil, err
		}
		packageName := goListJSON.ImportPath
		testFileDetailsByTest, err := getFileDetails(&goListJSON)
		if err != nil {
			return nil, err
		}
		testFileDetailByPackage[packageName] = testFileDetailsByTest
	}
	return testFileDetailByPackage, nil
}

func getTestDetails(packageName string) (testFileDetailsByTest, error) {
	var out bytes.Buffer
	var cmd *exec.Cmd
	stringReader := strings.NewReader("")
	cmd = exec.Command("go", "list", "-json", packageName)
	cmd.Stdin = stringReader
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	goListJSON := &goListJSON{}
	if err := json.Unmarshal(out.Bytes(), goListJSON); err != nil {
		return nil, err
	}
	return getFileDetails(goListJSON)
}

func getPackageDetails(allPackageNames map[string]*types.Nil) (testFileDetailsByPackage, error) {
	var testFileDetailByPackage testFileDetailsByPackage
	ctx := context.Background()
	g, ctx := errgroup.WithContext(ctx)
	details := make(chan testFileDetailsByPackage)
	for packageName := range allPackageNames {
		name := packageName
		g.Go(func() error {
			testFileDetailsByTest, err := getTestDetails(name)
			if err != nil {
				return err
			}
			select {
			case details <- testFileDetailsByPackage{name: testFileDetailsByTest}:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil

		})
	}
	go func() {
		g.Wait()
		close(details)
	}()

	testFileDetailByPackage = make(testFileDetailsByPackage, len(allPackageNames))
	for d := range details {
		for packageName, testFileDetailsByTest := range d {
			testFileDetailByPackage[packageName] = testFileDetailsByTest
		}
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return testFileDetailByPackage, nil
}

func getFileDetails(goListJSON *goListJSON) (testFileDetailsByTest, error) {
	testFileDetailByTest := map[string]*testFileDetail{}
	for _, file := range goListJSON.TestGoFiles {
		sourceFilePath := fmt.Sprintf("%s/%s", goListJSON.Dir, file)
		fileSet := token.NewFileSet()
		f, err := parser.ParseFile(fileSet, sourceFilePath, nil, 0)
		if err != nil {
			return nil, err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				testFileDetail := &testFileDetail{}
				fileSetPos := fileSet.Position(n.Pos())
				folders := strings.Split(fileSetPos.String(), "/")
				fileNameWithPos := folders[len(folders)-1]
				fileDetails := strings.Split(fileNameWithPos, ":")
				fmt.Println(fileDetails)
				lineNum, _ := strconv.Atoi(fileDetails[1])
				colNum, _ := strconv.Atoi(fileDetails[2])
				testFileDetail.FileName = fileDetails[0]
				testFileDetail.TestFunctionFilePos = testFunctionFilePos{
					Line: lineNum,
					Col:  colNum,
				}
				testFileDetailByTest[x.Name.Name] = testFileDetail
			}
			return true
		})
	}
	return testFileDetailByTest, nil
}

func checkIfStdinIsPiped() error {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return err
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return nil
	}
	return errors.New(color.Ize(color.Red, "error:") + "missing stdin pipe")
}

func generateReport(allTests map[string]*testStatus, testFileDetailByPackage testFileDetailsByPackage, elapsedTestTime time.Duration, discoveryFile string, executionFile string) error {
	f, err := os.Create(discoveryFile)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	f2, err := os.Create(executionFile)
	if err != nil {
		return err
	}
	defer f2.Close()
	w2 := bufio.NewWriter(f2)
	defer w2.Flush()

	var disc Discovery
	var exec Exec
	for _, test := range allTests {
		fmt.Println(test.TestFileName)
		name_hash := GetMD5Hash(test.TestName)
		var status string
		if test.Passed {
			status = "passed"
		} else {
			status = "failed"
		}
		exec.TestCase = append(exec.TestCase, TestCaseExec{ID: name_hash, Status: status, Duration: test.ElapsedTime})
		disc.TestSuites = append(disc.TestSuites, TestSuite{ID: name_hash, Label: test.TestName, Package: test.Package})
	}
	for _, teste := range allTests {
		file_name_hash := GetMD5Hash(teste.TestFileName)
		var suites []string
		for _, suite := range disc.TestSuites {
			if suite.Package == teste.Package {
				suites = append(suites, suite.ID)
			}
			var numTests int
			var failed int
			var passed int
			var status string
			init := 1
			for _, test := range allTests {
				var secondpkg string
				var pkg = test.Package
				if pkg == secondpkg {
					init++
					if test.Passed {
						passed++
					} else {
						failed++
					}
				}
				secondpkg = pkg
			}

			if failed > passed {
				status = "failed"
			} else {
				status = "passed"
			}

			exec.TestSuite = append(exec.TestSuite, TestSuiteExec{ID: suite.ID, Status: status, Duration: teste.ElapsedTime, NumTests: numTests, Package: teste.Package})
			disc.TestCases = append(disc.TestCases, TestCases{ID: file_name_hash, Label: teste.TestName, Package: teste.Package, TestSuites: suites})
		}
	}
	enc := json.NewEncoder(w2)
	enc.SetIndent("", "    ")
	if err := enc.Encode(exec); err != nil {
		return err
	}

	enc2 := json.NewEncoder(w)
	enc2.SetIndent("", "    ")
	if err := enc2.Encode(disc); err != nil {
		return err
	}
	return nil
}

func GetMD5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

func Exists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
