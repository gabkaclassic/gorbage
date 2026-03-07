package client

import (
	"fmt"
	"time"

	pb "github.com/gabkaclassic/gorbage/internal/proto"
)

func printFileInfo(fi *pb.FileInfo) {
	created := time.Unix(fi.GetCreatedAtUnix(), 0).Format("2006-01-02 15:04:05")

	fmt.Printf("Prefix:          %s\n", fi.GetPrefix())
	fmt.Printf("Original path:        %s\n", fi.GetOriginalPath())
	fmt.Printf("Size:        %s (%d bytes)\n", humanSize(fi.GetSize()), fi.GetSize())
	fmt.Printf("Created:     %s\n", created)
	fmt.Printf("Directory:   %t\n", fi.GetIsDirectory())
}

func printFileList(files []*pb.FileInfo) {
	if len(files) == 0 {
		fmt.Println("No files found")
		return
	}

	fmt.Printf("%-36s  %-8s  %-19s %-9s %s\n", "PREFIX", "SIZE", "CREATED", "Directory", "ORIGINAL PATH")
	for _, fi := range files {
		created := time.Unix(fi.GetCreatedAtUnix(), 0).Format("2006-01-02 15:04:05")
		fmt.Printf(
			"%-36s  %-8s  %-19s %-9t %s\n",
			fi.GetPrefix(),
			humanSize(fi.GetSize()),
			created,
			fi.GetIsDirectory(),
			fi.GetOriginalPath(),
		)
	}
}

func printDeleteResponse(resp *pb.DeleteResponse, fileID string) {
	if resp.GetSuccess() {
		fmt.Printf("File %s deleted successfully\n", fileID)
	} else {
		fmt.Printf("Failed to delete file %s\n", fileID)
	}
}

func humanSize(size uint64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%dB", size)
	}
	div, exp := uint64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB",
		float64(size)/float64(div),
		"KMGTPE"[exp],
	)
}
