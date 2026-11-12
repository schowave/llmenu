# Still for docs/budget.md and the website: budgets of the main key and the
# projects. Recorded by `mise run demo` together with docs/demo.tape.
Output docs/budget.gif # VHS needs a recording; demo.sh removes it

Set Shell bash
Set FontSize 16
Set Width 1320
Set Height 260
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms

# The budget needs a stored key, and the gateway URL confirmed after storing
# it (llmenu only sends a key to the gateway it was stored for).
Hide
Type `export HOME=$LLMENU_DEMO_HOME PS1='\w $ '; cd $HOME/acme; unset LLMENU_API_KEY; printf 'sk-dev\n' | llmenu secrets set gateway >/dev/null; llmenu config set gateway.url "$(llmenu config get gateway.url)" >/dev/null; clear`
Enter
Sleep 500ms
Show

Type "llmenu project list"
Enter
Sleep 2s
Screenshot docs/budget.png
Sleep 500ms
