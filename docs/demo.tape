# The README demo (GIF) and the website's video (MP4). Record with
# `mise run demo` (scripts/demo.sh sets up the fake gateway and environment
# first). It opens on the menu; the setup commands run hidden.
Output docs/demo.gif
Output site/demo.mp4

Set Shell bash
Set FontSize 14
Set Width 1440
Set Height 620
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms

Hide
Type "export HOME=$LLMENU_DEMO_HOME PS1='\w $ '; cd $HOME/acme"
Enter
Type "llmenu default claude >/dev/null; llmenu project use acme --here >/dev/null; clear"
Enter
Sleep 500ms
Show

Type "llmenu"
Sleep 300ms
Enter
Sleep 2.5s
Down
Sleep 800ms
Down
Sleep 800ms
Up
Up
Sleep 600ms
# Sort by Terminal-Bench, by price, and back to the Intelligence Index.
Tab
Sleep 1.5s
Tab
Sleep 1.5s
Tab
Sleep 800ms
Type "sonnet"
Sleep 1.5s
Enter
Sleep 3s
