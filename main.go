package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"gopkg.in/yaml.v3"
)

var workers = 10

func main() {
	log.SetPrefix("DEBUG ")
	log.SetFlags(0)
	defaultOutfile := fmt.Sprintf("%s-git-backup.tar.gz", time.Now().Format("2006-01-02"))

	outFilePtr := flag.String("out", defaultOutfile, "Output filename for the tarball")
	configFilePtr := flag.String("config", "codepack.yaml", "Configuration file")
	workersPtr := flag.Int("workers", 10, "Number of works for cloning repos")

	flag.Parse()

	username := os.Getenv("CODEPACK_GIT_USER")
	pass := os.Getenv("CODEPACK_GIT_PASS")
	workers = *workersPtr

	var auth *http.BasicAuth

	if username != "" && pass != "" {
		auth = &http.BasicAuth{
			Username: username,
			Password: pass,
		}
	}

	log.Println("Output File:", *outFilePtr)
	log.Println("Configuration File:", *configFilePtr)

	config, err := ConfigFromFile(*configFilePtr)
	if err != nil {
		panic(err)
	}

	outputFile, err := os.Create(*outFilePtr)
	if err != nil {
		panic(err)
	}

	tempDir, err := os.MkdirTemp(path.Join(os.TempDir()), "codepack")
	if err != nil {
		panic(err)
	}
	defer func() {
		// Clean up temp directory
		log.Println("Cleaning up temporary directory...")
		if err := os.RemoveAll(tempDir); err != nil {
			panic(err)
		}
	}()

	if err := cloneRepos(config, tempDir, auth); err != nil {
		panic(err)
	}

	if err := compress(tempDir, outputFile); err != nil {
		panic(err)
	}

}

func compress(src string, buf io.Writer) error {
	log.Println("Compressing files...")
	zr := gzip.NewWriter(buf)
	tw := tar.NewWriter(zr)

	filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		header.Name = filepath.ToSlash(filepath.Join("codepack", relPath))

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}
		return nil
	})

	if err := tw.Close(); err != nil {
		return err
	}

	if err := zr.Close(); err != nil {
		return err
	}

	return nil
}

func cloneRepos(config *Config, tempDir string, auth *http.BasicAuth) error {
	var wg sync.WaitGroup
	var failures atomic.Int32
	var successes atomic.Int32

	type request struct {
		url  string
		path string
	}
	resultChan := make(chan string)

	repoChan := make(chan request)

	for i := 0; i < int(math.Min(float64(workers), float64(len(config.Repos)))); i++ {
		go func() {
			for {
				req := <-repoChan
				resultChan <- fmt.Sprintf("Cloning %s to path %s", req.url, req.path)
				if err := bareMirrorClone(req.url, req.path, auth); err != nil {
					resultChan <- fmt.Sprintf("Cloning %s to path %s failed: %v", req.url, req.path, err)
					failures.Add(1)
					wg.Done()
					continue
				}
				successes.Add(1)
				wg.Done()
			}
		}()
	}

	// Log results from each goroutine
	go func() {
		for {
			msg := <-resultChan
			if msg == "done" {
				log.Println("Cloning complete")
				wg.Done()
				break
			}
			log.Println(msg)
		}
	}()

	for _, repo := range config.Repos {
		wg.Add(1)
		clonePath := path.Join(tempDir, repo.Path, repo.Name)
		repoChan <- request{url: repo.URL, path: clonePath}
	}

	wg.Wait()
	wg.Add(1)
	resultChan <- "done"
	// Wait for loging to be competed to avoid race condition
	wg.Wait()

	if failures.Load() != 0 {
		return fmt.Errorf("%d failure(s) cloning repositories, check log for details", failures.Load())
	}

	return nil
}

type Config struct {
	Repos []Repository `yaml:"repos"`
}

type Repository struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	Path string `yaml:"path"`
}

func ConfigFromFile(filename string) (*Config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	config := new(Config)

	err = yaml.NewDecoder(f).Decode(config)
	return config, err
}

func bareMirrorClone(url string, path string, auth *http.BasicAuth) error {
	_, err := git.PlainClone(path, true, &git.CloneOptions{
		URL:    url,
		Mirror: true,
		Auth:   auth,
	})

	return err
}
