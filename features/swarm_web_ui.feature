Feature: The swarm screen in the web UI
  The swarm screen (#/swarm) opens over the chat in a glass dock, with the
  shell's backdrop behind it. Below 1200px the shell stacks the rail into a
  top bar and the backdrop rises above the chat, so the dock has to rise with
  it, or every tap on the map, the search box or a node lands on the backdrop
  and closes the screen.

  Scenario: On a phone the swarm screen takes taps and clears the top bar
    Then the swarm dock sits above the shell's backdrop and below the top bar on a narrow shell

  Scenario: History origin filtering stays on the active swarm node
    Then the History origin filter applies to the node currently open through the relay without leaving it

  Scenario: The swarm canvas remembers a selected layout
    Then the swarm canvas starts as a tree and remembers the graph layout in this browser
