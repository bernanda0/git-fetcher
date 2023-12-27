package main

import (
	"encoding/csv"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

func main() {
	l := log.New(os.Stdout, "BR-", log.LstdFlags)

	file, err := os.Open("repos.csv")
	if err != nil {
		l.Fatal(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		l.Fatal(err)
	}

	var wg sync.WaitGroup
	cloneCh := make(chan string)

	err = godotenv.Load()
	if err != nil {
		l.Fatal("Error loading .env file")
	}

	username := os.Getenv("GITHUB_USERNAME")
	accessToken := os.Getenv("GITHUB_ACCESS_TOKEN")

	auth := &http.BasicAuth{
		Username: username,
		Password: accessToken,
	}

	date := os.Getenv("BEFORE_DATE")

	// Open the text file for writing
	outputFile, err := os.Create("TestedPackages.txt")
	if err != nil {
		l.Fatalf("Error creating text file: %v\n", err)
	}
	defer outputFile.Close()

	// Start Goroutines for cloning
	for _, record := range records {
		wg.Add(1)
		go cloneRepository(l, auth, record[0], record[1], record[2], cloneCh, &wg, date)
	}

	// Close the cloneCh channel once all cloning is done
	go func() {
		wg.Wait()
		close(cloneCh)
	}()

	// Move specific folders from each cloned repository
	for cloneDir := range cloneCh {
		moveSpecificFolders(l, cloneDir, outputFile)
	}

	l.Println("All repositories cloned and folders copied.")
}

func cloneRepository(l *log.Logger, auth *http.BasicAuth, repoURL, c, b string, cloneCh chan<- string, wg *sync.WaitGroup, targetTimeString string) {
	defer wg.Done()

	targetTime, err := time.Parse("2006-01-02 15:04:05", targetTimeString)
	if err != nil {
		l.Printf("Error parsing target time: %v\n", err)
		return
	}

	cloneDir := filepath.Join("./repo/", c)

	if _, err := os.Stat(cloneDir); err == nil {
		if err := os.RemoveAll(cloneDir); err != nil {
			l.Printf("Error deleting existing directory: %v\n", err)
			return
		}
	}
	l.Printf("Cloning %s into %s...\n", repoURL, cloneDir)

	var reference plumbing.ReferenceName
	branch := "refs/heads/" + b
	if b == "" {
		reference = ""
	} else {
		reference = plumbing.ReferenceName(branch)
	}

	l.Println("Cloning branch", reference)
	repo, err := git.PlainClone(cloneDir, false, &git.CloneOptions{
		Auth:          auth,
		URL:           repoURL,
		ReferenceName: reference,
		Progress:      os.Stdout,
	})
	if err != nil {
		l.Printf("Error cloning %s: %v\n", repoURL, err)
		return
	}

	// Fetch the latest commit before the target time
	headRef, err := repo.Head()
	if err != nil {
		l.Printf("Error getting HEAD reference: %v\n", err)
		return
	}

	var latestCommit *object.Commit
	commitIter, err := repo.Log(&git.LogOptions{From: headRef.Hash()})
	if err != nil {
		l.Printf("Error iterating commits: %v\n", err)
		return
	}
	commitIter.ForEach(func(c *object.Commit) error {
		if c.Committer.When.Before(targetTime) {
			latestCommit = c
			return object.ErrCanceled
		}
		return nil
	})

	if latestCommit != nil {
		l.Printf("Latest commit before %s in %s: %s\n", targetTime, c, latestCommit.Hash)

		// Checkout to the latest commit
		w, err := repo.Worktree()
		if err != nil {
			l.Printf("Error getting worktree: %v\n", err)
			return
		}
		// resetOptions := &git.ResetOptions{
		// 	Commit: latestCommit.Hash,
		// 	Mode:   git.HardReset,
		// }
		checkoutOptions := &git.CheckoutOptions{
			Hash:  latestCommit.Hash,
			Force: true,
		}
		err = w.Checkout(checkoutOptions)
		if err != nil {
			l.Printf("Error checking out: %v\n", err)
			return
		}

		cleanOptions := &git.CleanOptions{
			Dir: true,
		}

		w.Clean(cleanOptions)
		if err != nil {
			l.Printf("Error cleaning : %v\n", err)
			return
		}
		l.Println("Cleaning Success!")

		// err = w.Reset(resetOptions)
		// if err != nil {
		// 	l.Printf("Error resetting: %v\n", err)
		// 	return
		// }
		// l.Println("Reset Success!")

		// move to the folder
		// moveSpecificFolders(cloneDir)
	} else {
		l.Printf("No commits before %s in %s.\n", targetTime, c)
	}

	// Send the cloned directory to the channel
	cloneCh <- cloneDir
}

func moveSpecificFolders(l *log.Logger, cloneDir string, outputFile *os.File) {
	rootDir := "."
	written := false
	packageName := os.Getenv("PACKAGE_INFIX")
	movingDir := os.Getenv("MOVING_DIR")
	possiblePaths := []string{
		cloneDir, // buat modul sebelum intelliJ
		filepath.Join(cloneDir, "src", "main", "java", "com"), // buat modul after intellij
	}
	deleted := false

	for _, path := range possiblePaths {
		err := filepath.Walk(path, func(folderPath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if strings.Contains(info.Name(), packageName) && info.IsDir() {
				destPath := filepath.Join(rootDir, movingDir, info.Name())

				// Remove the destination directory if it exists
				if _, err := os.Stat(destPath); err == nil && !deleted {
					if err := os.RemoveAll(destPath); err != nil {
						l.Printf("Error removing destination directory: %v\n", err)
						return nil
					}
					deleted = true
				}

				// Ensure that the destination directory exists
				if err := os.MkdirAll(destPath, os.ModePerm); err != nil {
					l.Printf("Error creating destination directory: %v\n", err)
					return nil
				}

				// Copy the entire directory and its contents to the destination
				l.Printf("Copying %s to %s\n", folderPath, destPath)
				if err := copyDirectory(folderPath, destPath); err != nil {
					l.Printf("Error copying directory: %v\n", err)
					return nil
				}

				if !written {
					packageName := info.Name()

					if _, err := outputFile.WriteString(packageName + "\n"); err != nil {
						l.Printf("Error writing to text file: %v\n", err)
						return nil
					}
					l.Printf("Package '%s' written to text file.\n", packageName)
					written = true
				}

			}
			return nil
		})

		if err != nil {
			l.Println(err)
		}
	}
}

func copyDirectory(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dest, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := copyDirectory(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return err
	}

	return nil
}
