package main

import (
	"encoding/csv"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

func main() {
	file, err := os.Open("repos.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	cloneCh := make(chan string)

	err = godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Read the environment variables
	username := os.Getenv("GITHUB_USERNAME")
	accessToken := os.Getenv("GITHUB_ACCESS_TOKEN")
	auth := &http.BasicAuth{
		Username: username,
		Password: accessToken,
	}

	// Start Goroutines for cloning
	for _, record := range records {
		wg.Add(1)
		go cloneRepository(auth, record[0], record[1], cloneCh, &wg)
	}

	// Close the cloneCh channel once all cloning is done
	go func() {
		wg.Wait()
		close(cloneCh)
	}()

	// Move specific folders from each cloned repository
	for cloneDir := range cloneCh {
		moveSpecificFolders(cloneDir)
	}
}

func cloneRepository(auth *http.BasicAuth, repoURL, cloneDir string, cloneCh chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()

	if _, err := os.Stat(cloneDir); err == nil {
		if err := os.RemoveAll(cloneDir); err != nil {
			log.Printf("Error deleting existing directory: %v\n", err)
			return
		}
	}

	log.Printf("Cloning %s into %s...\n", repoURL, cloneDir)
	_, err := git.PlainClone(cloneDir, false, &git.CloneOptions{
		Auth:     auth,
		URL:      repoURL,
		Progress: os.Stdout,
	})
	if err != nil {
		log.Printf("Error cloning %s: %v\n", repoURL, err)
		return
	}

	// Send the cloned directory to the channel
	cloneCh <- cloneDir
}

func moveSpecificFolders(cloneDir string) {
	// Walk through the cloned directory
	err := filepath.Walk(cloneDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check if the path contains the desired package name
		if strings.Contains(path, "JBus") {
			destFolder := filepath.Join(cloneDir, "JBus")
			if info.IsDir() {
				// Move the folder to the destination
				newPath := filepath.Join(destFolder, info.Name())
				log.Printf("Moving %s to %s\n", path, newPath)
				err := os.Rename(path, newPath)
				if err != nil {
					log.Printf("Error moving folder: %v\n", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("Error walking through directory: %v\n", err)
	}
}
