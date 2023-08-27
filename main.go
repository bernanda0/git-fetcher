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

func cloneRepository(auth *http.BasicAuth, repoURL, c string, cloneCh chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()

	cloneDir := filepath.Join("./repo/", c)

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
	rootDir := "." // Set this to the path of your root directory if different

	// Define the possible paths where the "JSleep" directory might be located
	possiblePaths := []string{
		filepath.Join(cloneDir, "src", "main", "java", "com"),
		// Add more possible paths here if needed
	}

	for _, path := range possiblePaths {
		err := filepath.Walk(path, func(folderPath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Check if the folder contains "JSleep"
			if strings.Contains(info.Name(), "JSleep") && info.IsDir() {
				// Create the destination path in the "collect" folder
				destPath := filepath.Join(rootDir, "src", info.Name())

				// Ensure that the "collect" directory exists
				if err := os.MkdirAll(destPath, os.ModePerm); err != nil {
					log.Printf("Error creating 'collect' directory: %v\n", err)
					return nil
				}

				// Copy the entire directory and its contents to the destination
				log.Printf("Copying %s to %s\n", folderPath, destPath)
				if err := copyDirectory(folderPath, destPath); err != nil {
					log.Printf("Error copying directory: %v\n", err)
				}
			}
			return nil
		})

		if err != nil {
			log.Printf("Error searching for JSleep folder: %v\n", err)
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
