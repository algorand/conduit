package main

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"reflect"

	"golang.org/x/mod/modfile"
)

const maxTries = 10
const goMod = "go.mod"

func main() {
	var prevModFile, nextModFile *modfile.File
	var tryNum int
	for tryNum = 1; tryNum == 1 || tryNum <= maxTries; tryNum++ {
		if reflect.DeepEqual(prevModFile, nextModFile) {
			break
		}
		prevModFile = nextModFile
		var err error
		nextModFile, err = getAllAndTidyUp()
		if err != nil {
			panic(fmt.Sprintf("problem with go_mod_tidier round %d: %v", tryNum, err))
		}
	}
	if tryNum > maxTries {
		panic(fmt.Sprintf("go_mod_tidier tried %d times and still couldn't get a consistent result", maxTries))
	}
	fmt.Printf("Success! go_mod_tidier took %d tries to obtain a consistent result\n", tryNum-1)
}

func getAllAndTidyUp() (*modfile.File, error) {
	goModBytes, err := ioutil.ReadFile(goMod)
	if err != nil {
		return nil, fmt.Errorf("problem reading %s: %w", goMod, err)
	}

	mod, err := modfile.Parse(goMod, goModBytes, nil)
	if err != nil {
		return mod, fmt.Errorf("problem parsing %s: %w", goMod, err)
	}

	direct := make([]string, 0)
	for _, require := range mod.Require {
		if !require.Indirect {
			direct = append(direct, fmt.Sprintf("%s@%s", require.Mod.Path, require.Mod.Version))
		}
	}

	for _, pinnedPkg := range direct {
		fmt.Println("Getting", pinnedPkg)
		cmd := exec.Command("go", "get", pinnedPkg)
		err := cmd.Run()
		if err != nil {
			return mod, fmt.Errorf("problem with: go get %s: %w", pinnedPkg, err)
		}
	}

	cmdA := exec.Command("go", "mod", "tidy", fmt.Sprintf("-compat=%s", mod.Go.Version))
	err = cmdA.Run()
	if err != nil {
		return mod, fmt.Errorf("problem with: go mod tidy: %w", err)
	}

	return mod, nil
}
