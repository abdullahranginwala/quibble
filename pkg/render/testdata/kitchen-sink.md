# Kitchen Sink

A paragraph with **bold**, _italic_, ~~struck~~ text and an
[inline link](./other.md) plus an autolink to example.org.

## Features

Some prose introducing the feature table below.

| Feature | Status | Notes |
|---------|--------|-------|
| Parsing | done   | goldmark |
| Themes  | wip    | paper first |

### Task list

- [x] Write the parser
- [ ] Ship the themes
  - [ ] paper
  - [ ] ink

### Nested prose list

1. First item
   1. Nested one
   2. Nested two
2. Second item

#### Deep heading

> A blockquote spanning
> two source lines.
>
> And a second paragraph.

Here is a footnote reference.[^note]

---

## Code

Go:

```go
package main

func main() {
	println("hello")
}
```

Python:

```python
def greet(name):
    return f"hi {name}"
```

An unknown language falls back to plain text:

```wibble
!!! this is not a real language !!!
```

![a placeholder image](./img/diagram.png)

[^note]: The footnote body lives down here.
