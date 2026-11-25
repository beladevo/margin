# Regenerate with: vhs docs/demo.tape  (from the repo root)
Output docs/demo.gif

Set Shell bash
Set FontSize 16
Set Width 1300
Set Height 760
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms

Hide
Type "rm -rf /tmp/margin-demo && cp -r examples/notes /tmp/margin-demo && go build -o /tmp/margin-bin . && clear" Enter
Sleep 4s
Type "/tmp/margin-bin --config /dev/null /tmp/margin-demo" Enter
Sleep 1s
Show

Sleep 1s
Down@300ms 3
Sleep 2s
Screenshot docs/screenshots/view.png
Sleep 500ms

Up
Sleep 400ms
Enter
Sleep 400ms
Down@300ms 2
Sleep 2s
Screenshot docs/screenshots/table.png
Sleep 500ms

Type "e"
Sleep 500ms
Down@150ms 6
Ctrl+E
Enter
Type "4. Record a demo GIF"
Sleep 1s
Screenshot docs/screenshots/edit.png
Sleep 500ms
Escape
Sleep 1.5s

Ctrl+P
Sleep 400ms
Type "gosnip"
Sleep 1s
Screenshot docs/screenshots/finder.png
Sleep 500ms
Enter
Sleep 2s

Ctrl+F
Sleep 400ms
Type "bisect"
Sleep 1s
Screenshot docs/screenshots/search.png
Sleep 500ms
Enter
Sleep 2s

Type "?"
Sleep 2s
Screenshot docs/screenshots/help.png
Sleep 500ms
Type "x"
Sleep 500ms
Type "q"
Sleep 500ms
