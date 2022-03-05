package main

type Exec struct {
	TestCase  []TestCaseExec  `json:"testCase"`
	TestSuite []TestSuiteExec `json:"testSuite"`
}

type (
	TestCaseExec struct {
		ID         string  `json:"id"`
		Status     string  `json:"status"`
		Duration   float64 `json:"duration"`
		FailureMsg string  `json:"failureMsg"`
	}

	TestSuiteExec struct {
		ID         string  `json:"id"`
		Status     string  `json:"status"`
		Duration   float64 `json:"duration"`
		FailureMsg string  `json:"failureMsg"`
		Package    string  `json:"pkg"`
		NumTests   int     `json:"numTests"`
	}
)
type Discovery struct {
	TestCases  []TestCases `json:"testCases"`
	TestSuites []TestSuite `json:"testSuites"`
}

type TestCases struct {
	ID         string   `json:"id"`
	Label      string   `json:"label"`
	Package    string   `json:"pkg"`
	TestSuites []string `json:"test-suites"`
}

type TestSuite struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Package string `json:"pkg"`
}

type cmdFlags struct {
	titleFlag  string
	sizeFlag   string
	groupSize  int
	listFlag   string
	outputFlag string
	verbose    bool
	execFlag   string
}

type testStatus struct {
	TestName           string
	Package            string
	ElapsedTime        float64
	Output             []string
	Passed             bool
	Skipped            bool
	TestFileName       string
	TestFunctionDetail testFunctionFilePos
}

type goTestOutputRow struct {
	Time     string
	TestName string `json:"Test"`
	Action   string
	Package  string
	Elapsed  float64
	Output   string
}

type goListJSON struct {
	Dir         string
	ImportPath  string
	Name        string
	GoFiles     []string
	TestGoFiles []string
	Module      goListJSONModule
}

type goListJSONModule struct {
	Path string
	Dir  string
	Main bool
}

type (
	testFunctionFilePos struct {
		Line int
		Col  int
	}

	testFileDetail struct {
		FileName            string
		TestFunctionFilePos testFunctionFilePos
	}

	testFileDetailsByTest    map[string]*testFileDetail
	testFileDetailsByPackage map[string]testFileDetailsByTest
)
