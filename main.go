package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5"
	"gopkg.in/yaml.v3"
)

func main() {
	log.SetPrefix("DEBUG ")
	log.SetFlags(0)
	defaultOutfile := fmt.Sprintf("%s-git-backup.tar.gz", time.Now().Format("2006-01-02"))

	outFilePtr := flag.String("out", defaultOutfile, "Output filename for the tarball")
	configFilePtr := flag.String("config", "codepack.yaml", "Configuration file")

	flag.Parse()

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

	if err := cloneRepos(config, tempDir); err != nil {
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

func cloneRepos(config *Config, tempDir string) error {
	var wg sync.WaitGroup
	resultChan := make(chan string)
	var failures atomic.Int32

	for _, repo := range config.Repos {
		clonePath := path.Join(tempDir, repo.Path, repo.Name)
		log.Printf("Clone %s from %s to path %s\n", repo.Name, repo.URL, clonePath)
		wg.Add(1)

		go func(u string, p string) {
			defer wg.Done()
			if err := bareMirrorClone(u, p); err != nil {
				resultChan <- fmt.Sprintf("Cloning %s to path %s failed: %v", u, p, err)
				failures.Add(1)
				return
			}
			resultChan <- fmt.Sprintf("Cloned %s", u)
		}(repo.URL, clonePath)

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

func bareMirrorClone(url string, path string) error {
	_, err := git.PlainClone(path, true, &git.CloneOptions{
		URL:    url,
		Mirror: true,
	})

	return err
}
