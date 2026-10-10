package pier

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

func MeasureBytes(ctx context.Context, root string) (int64, error) {
	var size int64
	count := 0
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		count++
		if count > 100000 {
			return errors.New("pier measurement exceeds file limit")
		}
		if info.Mode().IsRegular() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
