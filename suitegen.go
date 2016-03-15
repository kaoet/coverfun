package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const kResultRoot = "/Users/kaoet/Desktop/result10"
const kAssertionCoverageRoot = "/Users/kaoet/Desktop/result11"
const kGroupSize = 100
const kOracleN = 5
const kAssertionCoverageN = 5

var kOracles = [kOracleN]float64{0.2, 0.4, 0.6, 0.8, 1.0}

var kSizes = []int{3, 10, 30, 100, 300}

type Result struct {
	mutant           string
	caseId           int
	crashed          bool
	output           string
	outputComponents []string
}

type TestCase struct {
	coverage   *Coverage
	ans        string
	utime      float64
	assertions []Assertion
}

type Assertion struct {
	coverage       *Coverage
	caseId         int
	index          int
	answer         string
	validateLength bool
}

type Coverage struct {
	expression []string
	branch     []string
	function   []string
	statement  []string
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU() * 2)

	dirs, err := ioutil.ReadDir(kResultRoot)
	if err != nil {
		panic(err)
	}
	for _, d := range dirs {
		if d.IsDir() && d.Name()[0] != '.' {
			err = process(d.Name())
			if err != nil {
				panic(err)
			}
		}
	}
}

func process(name string) error {
	fmt.Println("====Processing", name)
	root := path.Join(kResultRoot, name)
	os.Remove(path.Join(root, "coverage-effectiveness-raw-dots.csv"))
	os.Remove(path.Join(root, "coverage-effectiveness-normalized-dots.csv"))
	//os.Remove(path.Join(root, "oracle-size-effectiveness-dots.csv"))
	//os.Remove(path.Join(root, "assertion-coverage-effectiveness-dots.csv"))
	result, err := readResult(path.Join(root, "result.csv"))
	if err != nil {
		return err
	}
	cases, err := readCases(path.Join(root, "test cases"))
	if err != nil {
		return err
	}
	allBrch, allStmt, allExpr, err := readAllCov(path.Join(root, "test cases"))
	if err != nil {
		return err
	}

	killableMutantN := countMutant(result)

	fmt.Println("separate cases")
	nAssertions := 0
	for caseId := range cases {
		nAssertions++
		cases[caseId].assertions = []Assertion{{caseId: caseId, coverage: cases[caseId].coverage, validateLength: true}}
		for i, comp := range separateComponents(cases[caseId].ans) {
			nAssertions++
			assertion := Assertion{caseId: caseId, index: i, answer: comp}
			assertion.coverage = readAssertionCoverage(path.Join(kAssertionCoverageRoot, name), caseId, i)
			if assertion.coverage == nil {
				assertion.coverage = cases[caseId].coverage
			}
			cases[caseId].assertions = append(cases[caseId].assertions, assertion)
		}
	}

	fmt.Println("nAssertions = ", nAssertions)
	/*
		fmt.Println("separate results")
		for rIdx := range result {
			result[rIdx].outputComponents = separateComponents(result[rIdx].output)
		}
	*/
	_, _, _, _ = allBrch, allStmt, allExpr, killableMutantN

	// 1. coverge vs. effectiveness
	{
		fmt.Println("coveredMutants")
		coveredMutants, err := readMutants(path.Join(root, "mutants"))
		if err != nil {
			return err
		}

		fmt.Println("sameSizeGroups")
		groups := sameSizeGroups(len(cases), kSizes)

		fmt.Println("calcEffectiveness")
		rawEff, normEff := calcEffectiveness(groups, result, cases, coveredMutants, killableMutantN)

		fmt.Println("calcCoverage")
		brch, stmt, expr := calcCoverage(allBrch, allStmt, allExpr, groups, cases)

		fmt.Println("writeDots")
		err = writeCoverageEffectiveness(rawEff, brch, stmt, expr, path.Join(root, "coverage-effectiveness-raw-dots.csv"))
		if err != nil {
			return err
		}

		err = writeCoverageEffectiveness(normEff, brch, stmt, expr, path.Join(root, "coverage-effectiveness-normalized-dots.csv"))
		if err != nil {
			return err
		}
	}
	/*
		// 2. assertion% vs. effectiveness
		{
			groups := randomSizeGroups(len(cases))

			for oid := range kOracles {
				fmt.Println("oracle:", kOracles[oid])

				fmt.Println("calcEffectiveness")
				eff := calcRandomEffectiveness(groups, cases, result, kOracles[oid], killableMutantN)

				fmt.Println("writeDots")
				err = writeOracleEffectiveness(groups, oid, eff, path.Join(root, "oracle-size-effectiveness-dots.csv"))
				if err != nil {
					return err
				}
			}
		}

		// 3. assertion coverage vs. effectiveness
		{
			fmt.Println("collect assertions")
			assertions := []Assertion{}
			for _, c := range cases {
				assertions = append(assertions, c.assertions...)
			}

			fmt.Println("max coverage level")
			maxCov := maxCoverage(cases, allBrch)

			fmt.Println("generating test suites")
			groups := [][]int{}
			coverages := []float64{}
			for i := 1; i <= kAssertionCoverageN; i++ {
				targetCoverage := float64(i) * (maxCov / kAssertionCoverageN)
				x, y := suitesWithTargetCoverage(assertions, targetCoverage, allBrch)
				groups = append(groups, x[:]...)
				coverages = append(coverages, y[:]...)
			}

			fmt.Println("calc effectiveness")
			eff := calcAssertionEffectiveness(assertions, groups, cases, result, killableMutantN)

			fmt.Println("writeDots")
			err = writeAssertionCoverageEffectiveness(eff, coverages, path.Join(root, "assertion-coverage-effectiveness-dots.csv"))
			if err != nil {
				return err
			}
		}
	*/
	return nil
}

func randomSizeGroups(caseN int) (ret [][]int) {
	ret = make([][]int, kGroupSize)
	for i := range ret {
		ret[i] = suiteWithSize(caseN, rand.Intn(caseN))
	}
	return
}
func maxCoverage(cases []TestCase, allBrch []string) float64 {
	haveBrch := make(map[string]bool)
	for _, c := range cases {
		for _, s := range c.coverage.branch {
			haveBrch[s] = true
		}
	}
	return float64(len(haveBrch)) / float64(len(allBrch))
}

func suitesWithTargetCoverage(assertions []Assertion, targetCoverage float64, allBrch []string) (groups [kGroupSize][]int, cov [kGroupSize]float64) {
	for gIdx := range groups {
		haveBrch := make(map[string]bool)
		selectedAssertions := []int{}
		for _, idx := range rand.Perm(len(assertions)) {
			var newN int
			for _, item := range assertions[idx].coverage.branch {
				if !haveBrch[item] {
					newN++
				}
			}
			if newN > 0 && float64(newN+len(haveBrch)) <= targetCoverage*float64(len(allBrch)) {
				selectedAssertions = append(selectedAssertions, idx)
				for _, item := range assertions[idx].coverage.branch {
					haveBrch[item] = true
				}
			}
		}
		groups[gIdx] = selectedAssertions
		cov[gIdx] = float64(len(haveBrch)) / float64(len(allBrch))
	}
	return
}

func calcAssertionEffectiveness(assertions []Assertion, groups [][]int, cases []TestCase, result []Result, killableMutantN int) (raw []float64) {
	raw = make([]float64, len(groups))
	for i := range groups {
		haveCaseId := make(map[int]bool)
		killedMutant := make(map[string]bool)
		for _, assertionId := range groups[i] {
			haveCaseId[assertions[assertionId].caseId] = true
		}

		for _, r := range result {
			if haveCaseId[r.caseId] {
				for _, a := range assertions {
					if r.caseId == a.caseId && a.kills(cases, r) {
						killedMutant[r.mutant] = true
					}
				}
			}
		}

		raw[i] = float64(len(killedMutant)) / float64(killableMutantN)
	}
	return
}

func writeCoverageEffectiveness(eff, brch, stmt, expr [][kGroupSize]float64, file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()
	writer := csv.NewWriter(f)
	err = writer.Write([]string{"group id", "effectiveness", "statement", "branch", "expression"})
	if err != nil {
		return err
	}
	for i := range eff {
		for j := 0; j < kGroupSize; j++ {
			err = writer.Write([]string{
				strconv.Itoa(i),
				fmt.Sprint(eff[i][j]),
				fmt.Sprint(stmt[i][j]),
				fmt.Sprint(brch[i][j]),
				fmt.Sprint(expr[i][j]),
			})
		}
	}
	writer.Flush()
	if err = writer.Error(); err != nil {
		return err
	}
	return nil
}
func writeAssertionCoverageEffectiveness(eff, coverage []float64, file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()
	writer := csv.NewWriter(f)
	err = writer.Write([]string{"coverage", "effectiveness"})
	if err != nil {
		return err
	}
	for i := range eff {
		err = writer.Write([]string{
			fmt.Sprint(coverage[i]),
			fmt.Sprint(eff[i]),
		})
	}
	writer.Flush()
	if err = writer.Error(); err != nil {
		return err
	}
	return nil
}
func writeOracleEffectiveness(groups [][]int, oid int, eff []float64, file string) error {
	_, err := os.Stat(file)
	if err != nil {
		if os.IsNotExist(err) {
			err = ioutil.WriteFile(file, []byte("size,oid,effectiveness\n"), 0644)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	f, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	writer := csv.NewWriter(f)
	for i := range eff {
		err = writer.Write([]string{
			strconv.Itoa(len(groups[i])),
			strconv.Itoa(oid),
			fmt.Sprint(eff[i]),
		})
	}
	writer.Flush()
	if err = writer.Error(); err != nil {
		return err
	}
	return nil
}
func readResult(path string) ([]Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := csv.NewReader(f) // mutant, case id, crashed, killed, output
	lines, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var ret []Result
	for i := 1; i < len(lines); i++ {
		if lines[i][3] == "yes" { // only killed
			caseId, err := strconv.ParseInt(lines[i][1], 10, 0)
			if err != nil {
				return nil, err
			}
			if lines[i][2] == "yes" {
				ret = append(ret, Result{
					mutant:  lines[i][0],
					caseId:  int(caseId),
					crashed: true,
					output:  lines[i][4],
				})
			} else {
				ret = append(ret, Result{
					mutant:  lines[i][0],
					caseId:  int(caseId),
					crashed: false,
					output:  lines[i][4],
				})
			}
		}
	}
	return ret, nil
}
func readLines(path string) ([]string, error) {
	var ret []string
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		if text != "" {
			ret = append(ret, text)
		}
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}
func readAssertionCoverage(root string, caseId, compId int) *Coverage {
	name := "case_" + strconv.Itoa(caseId) + "_" + strconv.Itoa(compId)
	fn, err := readLines(path.Join(root, name+".function"))
	if err != nil {
		return nil
	}
	stmt, err := readLines(path.Join(root, name+".statement"))
	if err != nil {
		return nil
	}
	brch, err := readLines(path.Join(root, name+".branch"))
	if err != nil {
		return nil
	}
	expr, err := readLines(path.Join(root, name+".expression"))
	if err != nil {
		return nil
	}
	return &Coverage{
		function:   fn,
		statement:  stmt,
		branch:     brch,
		expression: expr,
	}
}
func readCases(root string) ([]TestCase, error) {
	var ret []TestCase
	for i := 0; ; i++ {
		si := strconv.Itoa(i)
		fn, err := readLines(path.Join(root, "case_"+si+".function"))
		if err != nil {
			if os.IsNotExist(err) {
				return ret, nil
			}
			return nil, err
		}
		stmt, err := readLines(path.Join(root, "case_"+si+".statement"))
		if err != nil {
			return nil, err
		}
		brch, err := readLines(path.Join(root, "case_"+si+".branch"))
		if err != nil {
			return nil, err
		}
		expr, err := readLines(path.Join(root, "case_"+si+".expression"))
		if err != nil {
			return nil, err
		}
		ans, err := ioutil.ReadFile(path.Join(root, "case_"+si+".answer"))
		if err != nil {
			return nil, err
		}
		utime, err := ioutil.ReadFile(path.Join(root, "case_"+si+".utime"))
		if err != nil {
			return nil, err
		}
		utimeFloat, err := strconv.ParseFloat(string(utime), 64)
		if err != nil {
			return nil, err
		}
		ret = append(ret, TestCase{
			coverage: &Coverage{
				function:   fn,
				statement:  stmt,
				expression: expr,
				branch:     brch,
			},
			ans:   string(ans),
			utime: utimeFloat,
		})
	}
}
func readAllCov(root string) ([]string, []string, []string, error) {
	brch, err := readLines(path.Join(root, "all.branch"))
	if err != nil {
		return nil, nil, nil, err
	}
	stmt, err := readLines(path.Join(root, "all.statement"))
	if err != nil {
		return nil, nil, nil, err
	}
	expr, err := readLines(path.Join(root, "all.expression"))
	if err != nil {
		return nil, nil, nil, err
	}
	return brch, stmt, expr, nil
}
func readMutants(root string) (ret map[string][]string, err error) {
	ret = make(map[string][]string)
	err = filepath.Walk(root, func(filePath string, f os.FileInfo, err error) error {
		if err != nil || f == nil {
			return err
		}
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".fn") {
			var bytes []byte
			bytes, err = ioutil.ReadFile(filePath)
			if err != nil {
				return err
			}
			fn := string(bytes)
			ret[fn] = append(ret[fn], strings.TrimPrefix(strings.TrimSuffix(filePath, ".fn"), root+"/"))
		}
		return nil
	})
	return
}
func lex(output []rune) (string, []rune) {
	switch output[0] {
	case '(', ')', ',', '[', ']':
		return string(output[0]), output[1:]
	case '"', '\'':
		i := 1
		for ; i < len(output) && output[i] != output[0]; i++ {
		}
		if i < len(output) {
			return string(output[:i+1]), output[i+1:]
		} else {
			return string(output), nil
		}
	case ' ':
		i := 1
		for ; i < len(output) && output[i] == ' '; i++ {
		}
		return string(output[:i]), output[i:]
	default:
		i := 1
		for ; i < len(output) && strings.IndexRune(" []()'\",", output[i]) == -1; i++ {
		}
		return string(output[:i]), output[i:]
	}
}
func parseE(tokens []string, inSpace bool) (components []string, leftOver []string) {
	if len(tokens) == 0 {
		return nil, nil
	}
	switch tokens[0] {
	case "(", "[":
		before := tokens[1:]
		closeSign := ")"
		if tokens[0] == "[" {
			closeSign = "]"
		}
		for len(before) > 0 && before[0] != closeSign {
			_, after := parseE(before, false)
			components = append(components, strings.Join(before[:len(before)-len(after)], ""))
			if len(after) > 0 && after[0] == "," {
				before = after[1:]
			} else {
				before = after
			}
		}
		if len(before) > 0 {
			before = before[1:]
			if !inSpace && len(before) > 0 && before[0] == " " {
				return parseEE(before, strings.Join(tokens[:len(tokens)-len(before)], ""))
			} else {
				leftOver = before
				return
			}
		} else {
			leftOver = before
			return
		}
	default:
		if !inSpace && len(tokens) > 1 && tokens[1] == " " {
			return parseEE(tokens[1:], tokens[0])
		} else {
			return []string{tokens[0]}, tokens[1:]
		}
	}
}
func parseEE(tokens []string, first string) (components []string, leftOver []string) {
	components = []string{first}
	before := tokens
	for len(before) > 0 && before[0] == " " {
		before = before[1:]
		_, after := parseE(before, true)
		components = append(components, strings.Join(before[:len(before)-len(after)], ""))
		before = after
	}
	leftOver = before
	return
}
func separateComponents(answer string) []string {
	tokens := []string{}
	output := []rune(answer)
	for len(output) > 0 {
		var token string
		token, output = lex(output)
		tokens = append(tokens, token)
	}
	comp, _ := parseE(tokens, false)
	return comp
}
func (a Assertion) kills(cases []TestCase, r Result) bool {
	if a.validateLength {
		if len(r.outputComponents) != len(cases[a.caseId].assertions) {
			return true
		}
	} else {
		if !(len(r.outputComponents) > a.index && r.outputComponents[a.index] == a.answer) {
			return true
		}
	}
	return false
}
func calcRandomEffectiveness(groups [][]int, cases []TestCase, result []Result, oracle float64, killableMutantN int) (raw []float64) {
	raw = make([]float64, len(groups))
	for i := range groups {
		haveCaseId := make(map[int]bool)
		killedMutant := make(map[string]bool)
		for _, caseId := range groups[i] {
			haveCaseId[caseId] = true
		}

		assertions := []Assertion{}
		for _, caseId := range groups[i] {
			assertions = append(assertions, cases[caseId].assertions...)
		}
		seq := rand.Perm(len(assertions))
		limit := int(float64(len(assertions)) * oracle)

		for _, r := range result {
			if r.crashed {
				continue
			}

			if haveCaseId[r.caseId] {
				for k := 0; k < limit; k++ {
					ass := assertions[seq[k]]
					if r.caseId == ass.caseId && ass.kills(cases, r) {
						killedMutant[r.mutant] = true
						break
					}
				}
			}
		}

		raw[i] = float64(len(killedMutant)) / float64(killableMutantN)
	}
	return
}

func suiteWithSize(caseN, size int) (ret []int) {
	ret = make([]int, size)
	haveChosen := make(map[int]bool)

	for k := 0; k < size; k++ {
		p := caseN - size + k
		q := rand.Intn(p + 1)
		if haveChosen[q] {
			ret[k] = p
			haveChosen[p] = true
		} else {
			ret[k] = q
			haveChosen[q] = true
		}
	}
	return
}
func sameSizeGroups(caseN int, sizes []int) (ret [][kGroupSize][]int) {
	ret = make([][kGroupSize][]int, len(sizes))
	for i, size := range sizes {
		if caseN < size {
			return ret[:i]
		}
		for j := 0; j < kGroupSize; j++ {
			ret[i][j] = suiteWithSize(caseN, size)
		}
	}
	return
}
func randomSuites(caseN int, count int) (ret [][]int) {
	ret = make([][]int, count)
	for i := range ret {
		ret[i] = suiteWithSize(caseN, rand.Intn(caseN)+1)
	}
	return
}
func countMutant(result []Result) int {
	haveMutant := make(map[string]bool)
	for _, r := range result {
		haveMutant[r.mutant] = true
	}
	return len(haveMutant)
}
func calcEffectiveness(groups [][kGroupSize][]int, result []Result, cases []TestCase, coveredMutants map[string][]string, killableMutantN int) (raw, normalized [][kGroupSize]float64) {
	raw = make([][kGroupSize]float64, len(groups))
	normalized = make([][kGroupSize]float64, len(groups))
	killable := make(map[string]bool)
	for _, r := range result {
		killable[r.mutant] = true
	}
	for i := range groups {
		for j := 0; j < kGroupSize; j++ {
			haveCaseId := make(map[int]bool)
			haveFn := make(map[string]bool)
			killedMutant := make(map[string]bool)
			coveredMutant := make(map[string]bool)
			for _, caseId := range groups[i][j] {
				haveCaseId[caseId] = true
				for _, fn := range cases[caseId].coverage.function {
					haveFn[fn] = true
				}
			}
			for fn := range haveFn {
				for _, mu := range coveredMutants[fn] {
					if killable[mu] {
						coveredMutant[mu] = true
					}
				}
			}
			for _, r := range result {
				if haveCaseId[r.caseId] {
					killedMutant[r.mutant] = true
					if !coveredMutant[r.mutant] {
						fmt.Println("Warning! Killed but not covered:", r.mutant)
						coveredMutant[r.mutant] = true
					}
				}
			}
			raw[i][j] = float64(len(killedMutant)) / float64(killableMutantN)
			if len(coveredMutant) == 0 {
				normalized[i][j] = 0
			} else {
				normalized[i][j] = float64(len(killedMutant)) / float64(len(coveredMutant))
			}
		}
	}
	return
}
func calcCoverage(allBrch, allStmt, allExpr []string, groups [][kGroupSize][]int, cases []TestCase) (brch, stmt, expr [][kGroupSize]float64) {
	brch = make([][kGroupSize]float64, len(groups))
	expr = make([][kGroupSize]float64, len(groups))
	stmt = make([][kGroupSize]float64, len(groups))

	type IJ struct{ i, j int }
	ijs := make(chan IJ)
	done := make(chan int)

	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			for ij := range ijs {
				i, j := ij.i, ij.j
				haveStmt := make(map[string]bool)
				haveExpr := make(map[string]bool)
				haveBrch := make(map[string]bool)
				for _, caseId := range groups[i][j] {
					for _, stmt := range cases[caseId].coverage.statement {
						haveStmt[stmt] = true
					}
					for _, brch := range cases[caseId].coverage.branch {
						haveBrch[brch] = true
					}
					for _, expr := range cases[caseId].coverage.expression {
						haveExpr[expr] = true
					}
				}
				stmt[i][j] = float64(len(haveStmt)) / float64(len(allStmt))
				brch[i][j] = float64(len(haveBrch)) / float64(len(allBrch))
				expr[i][j] = float64(len(haveExpr)) / float64(len(allExpr))
			}
			done <- 1
		}()
	}

	for i := range groups {
		for j := 0; j < kGroupSize; j++ {
			ijs <- IJ{i, j}
		}
	}
	close(ijs)

	for i := 0; i < runtime.NumCPU(); i++ {
		<-done
	}
	return
}
