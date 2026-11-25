# Go snippets

## Atomic file write

```go
tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
if err != nil {
	return err
}
defer os.Remove(tmp.Name())
if _, err := tmp.WriteString(content); err != nil {
	return err
}
if err := tmp.Close(); err != nil {
	return err
}
return os.Rename(tmp.Name(), path)
```

## Table-driven test

```go
for _, tc := range cases {
	t.Run(tc.name, func(t *testing.T) {
		if got := f(tc.in); got != tc.want {
			t.Fatalf("got %v, want %v", got, tc.want)
		}
	})
}
```
