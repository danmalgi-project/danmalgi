package rootdir

import (
	"os"
	"path/filepath"
)

// GetProjectRoot 함수는 go.mod 파일을 찾아서 프로젝트 루트를 반환합니다.
func Get() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "../.."
	}

	currentDir := cwd
	for {
		if _, err := os.Stat(filepath.Join(currentDir, "go.mod")); err == nil {
			return currentDir
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir { // 루트 디렉토리에 도달
			break
		}
		currentDir = parentDir
	}
	return "../.."
}
