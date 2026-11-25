# Git cheatsheet

## Undo the last commit, keep the changes

```sh
git reset --soft HEAD~1
```

## Find which commit broke something

```sh
git bisect start
git bisect bad
git bisect good v1.2.0
```

## Clean up merged branches

```sh
git branch --merged main | grep -v main | xargs git branch -d
```
