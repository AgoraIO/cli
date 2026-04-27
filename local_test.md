There's already an agora at your nvm Node path (the TypeScript version). I'll note the conflict and show both approaches.       
   
  ---                                                                                                                             
  Local Testing Guide                                                                     
                                                                                                                                  
  Build                                                                                                                           
                                                                                                                                  
  cd /Users/arlene/Agora/devex/github/cli-workspace/agora-cli-go                                                                  
  go build -o ./agora .                                                                                                           
   
  This is the only command you need to run after any code change — the binary is rebuilt in-place.                                
                                                                                                                                  
  ---                                                                                                                             
  Option A — Run directly (no install, no PATH conflict)                                                                          
                                                                                                                                  
  Best when you're iterating quickly and don't care about the install path:
                                                                                                                                  
  ./agora --help                                            
  ./agora --help --all                                                                                                            
  ./agora --help --all --json                               
  ./agora login --no-browser
  ./agora whoami

  ---                                                                                                                             
  Option B — Symlink install (global, shadows the npm version)
                                                                                                                                  
  Your current agora resolves to the npm/Node version at:   
  /Users/arlene/.nvm/versions/node/v22.20.0/bin/agora                                                                             
                                                     
  To shadow it, symlink into ~/go/bin (your GOPATH bin), which should be — or can be — earlier in PATH than nvm:                  
                                                                                                                                  
  # One-time: ensure ~/go/bin is in PATH before nvm in your ~/.zshrc
  # Add this line BEFORE the nvm init block:                                                                                      
  # export PATH="$HOME/go/bin:$PATH"                                                                                              
                                                                                                                                  
  # Install (re-run after each go build if the binary path changes, but                                                           
  # since we're symlinking to the repo copy, go build updates it in-place)                                                        
  ln -sf "$(pwd)/agora" ~/go/bin/agora                                                                                            
                                                            
  # Verify it resolved to the Go build, not the npm one                                                                           
  which agora          # should show ~/go/bin/agora         
  agora --version                                                                                                                 
                                                            
  If you don't want to touch your shell config, use /usr/local/bin instead — it's already present and typically before nvm on     
  macOS Intel:                                              
                                                                                                                                  
  sudo ln -sf "$(pwd)/agora" /usr/local/bin/agora           
  which agora          # should show /usr/local/bin/agora
                                                                                                                                  
  ---
  Rebuild cycle                                                                                                                   
                                                            
  go build -o ./agora .   # rebuilds; symlink picks up the change automatically
  agora --version          # confirm new binary is live                                                                           
                                                                                                                                  
  No re-linking needed — the symlink points to the repo path and the build overwrites it.                                         
                                                                                                                                  
  ---                                                                                                                             
  Test the new help features                                

  # Human-readable deep help
  agora --help --all                                                                                                              
   
  # Machine-readable (agent tooling)                                                                                              
  agora --help --all --json                                 
                                                                                                                                  
  # Pipe to jq for exploring
  agora --help --all --json | jq '.commands[] | select(.flags | length > 0)'                                                      
  agora --help --all --json | jq '[.commands[].path]'                                                                             
   
  ---                                                                                                                             
  Remove / restore the npm version                          
                                                                                                                                  
  # Remove the symlink
  rm ~/go/bin/agora          # if you used ~/go/bin                                                                               
  # or                                                                                                                            
  sudo rm /usr/local/bin/agora   # if you used /usr/local/bin
                                                                                                                                  
  # Verify npm version is restored                          
  which agora   # should show the nvm path again                                                                                  
  agora --version                                                                                                                 
   
  ---                                                                                                                             
  The key property of the symlink approach: go build -o ./agora . updates the binary at the symlink target, so the installed
  command is always in sync with the source after a build — no reinstall step.  