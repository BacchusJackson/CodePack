# CodePack

A CLI tool to create local backups for multiple git repositories

## Instructions

`name` will be the name of the project and the directory containing the code after packing

`path` will be the path that will be created in the archive

`url` the url to the target git repository

CodePack supports basic authentication via Environment variables

`CODEPACK_GIT_USER`: The username for git, if using a GitHub token, username should be `token`

`CODEPACK_GIT_PASS`: the password / token for git

```yaml 
repos:
  - name: grype
    path: tools
    url: "https://github.com/anchore/grype.git"
  - name: semgrep
    path: tools
    url: "https://github.com/returntocorp/semgrep.git"
  - name: cobra
    path: development
    url: "https://github.com/spf13/cobra.git"
```

```bash
Usage of codepack:

  -config string
        Configuration file (default "codepack.yaml")
  -log string
        optional log file for log output
  -out string
        Output filename for the tarball (default "2023-06-16-git-backup.tar.gz")
  -skiptar
        do not tarball and compress codepack content
  -version
        output version information and exit
  -workers int
        Number of works for cloning repos (default 10)
```

```bash
codepack -config mycodepack.yaml -out "my-backups.tar.gz"
```

this will produce a gzipped tarball that can be extracted with tar if necessary

```bash
tar xf 2023-06-14-backup.tar.gz
```

The resulting directory structure after extraction for the example would be 

```
codepack
|_ tools
   |_ grype
   |_ semgrep
|_ deveopment
   |_ cobra
```

these are bare, mirrored repositories which can be used with worktrees or cloned to another directory

```bash
mkdir somedir
git clone --all file://<full path to codepath>/tools/grype somedir/grype
cd somedir/grype
git branch
git status
```

Using worktrees:

```bash
cd codepack/tools/grype
git worktree add main main
cd main
git status
```

Using worktrees will create a folder named `main` with the `main` branch checkout in that directory

