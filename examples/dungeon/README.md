# Cave of Cards

A small directed-graph adventure with cycles, convergence, and one piece of state

## Walkthrough

### Welcome, adventurer

You came for the treasure. Or the glory. Or because the rent is due
and the dragon hoard is famously well-funded.

Make your choices. Try not to die. If you do die, the cave is
suspiciously generous about second chances.

### 1. The entrance

The cave mouth yawns dark. A breeze stirs the tall grass behind you.
Three paths lead onward. Your boots crunch on something that might
have been a previous adventurer.

### 2. A formal introduction

The cave breeze coils around you. It seems to want a formal greeting
before letting you any further in.

**Inputs:**

- `name` = `Wanderer`

```
(The cave murmurs: "Welcome, Wanderer the Foolhardy.")
```

### 3. Listen for a moment

The cave is full of small sounds. If you stand still and listen,
you might catch a hint of what waits inside.

(Press Enter to move on whenever you're ready.)

```
[38;5;245m    drip... drip...[0m
[38;5;245m    (faint scrabbling — claws on stone, somewhere west)[0m
[38;5;245m    (a slow leathery breath, deep in the rock)[0m
[38;5;245m    (coins shifting; the dragon is restless tonight)[0m
[38;5;245m    (a dragonfly bumps into a stalactite and apologizes)[0m
[2m(The cave seems to forget about you.)[0m
```

### 4. Which way?

A faded sign reads: NORTH — passages, SOUTH — meadow, EAST — see manager.

**Inputs:**

- `direction` = `north`

→ jumped to `cave-fork`

### 5. The forked passage

The northern passage forks. The left tunnel is dark and silent.
The right tunnel has cool air and faint scratching noises.

**Inputs:**

- `fork` = `left`

→ jumped to `dark-passage`

### 6. The dark passage

You stumble through the dark. Your hand finds wet stone. Somewhere
ahead, something that is definitely not a small mouse is breathing.

**Inputs:**

- `nerve` = `forward`

→ jumped to `dragon`

### 7. The dragon's lair

A red dragon reclines on a heap of gold, flipping through a magazine
titled *Hoard Quarterly*. It glances up at you.

"Oh good. Lunch."

(The reveal streams line-by-line — press Enter at any point to
skip ahead to the verdict.)

```
[38;5;245m      The cave widens. Your torch flickers and gives up entirely.[0m
[38;5;245m      Something else takes over the lighting.[0m

[36m                    /\        /\        /\        /\[0m
[36m                   /  \      /  \      /  \      /  \[0m
[36m                  /    \    /    \    /    \    /    \[0m
[36m                 /______\  /______\  /______\  /______\[0m

[38;5;88m                              \||/[0m
[38;5;88m                              |  [1;33m@[38;5;88m___oo[0m
[38;5;88m                    /\  /\   / (__,,,,|[0m
[38;5;88m                   ) /^\) ^\/ _)[0m
[38;5;88m                   )   /^\/   _)[0m
[38;5;88m                   )   _ /  / _)[0m
[38;5;88m                  /\  )/\/ ||  | )_)[0m
[38;5;88m                 <  >      |(,,) )__)[0m
[38;5;88m                  ||      /    \)___)\[0m
[38;5;88m                  | \____(      )___) )___[0m
[38;5;88m                   \______(_______;;; __;;;[0m

[38;5;220m      Around him: rivers of gold, sparkling like dirty fountains.[0m
[2m      A magazine titled "[0m[1mHoard Quarterly[0m[2m" rests across one claw.[0m
[2m      He licks a page with a forked tongue and turns it.[0m
[2m      The article is a six-page spread on adamantine polishing.[0m

[1;33m      Slowly, two enormous yellow eyes lift to meet yours.[0m

[1;31m      "Oh good,"[0m[31m he says, conversationally. [1;31m"Lunch."[0m
```

→ jumped to `death`

### 8. You died (sort of)

The dragon yawns, exhales, and you become a statistic.

Some time passes. You wake up at the cave entrance with no memory of
dying. Your boots crunch on something that might have been a previous
adventurer.

→ jumped to `entrance`

### 9. The entrance _(visit 2)_

The cave mouth yawns dark. A breeze stirs the tall grass behind you.
Three paths lead onward. Your boots crunch on something that might
have been a previous adventurer.

### 10. A formal introduction _(visit 2)_

The cave breeze coils around you. It seems to want a formal greeting
before letting you any further in.

**Inputs:**

- `name` = `Wanderer`

```
(The cave murmurs: "Back so soon, Wanderer the Foolhardy?")
```

### 11. Listen for a moment _(visit 2)_

The cave is full of small sounds. If you stand still and listen,
you might catch a hint of what waits inside.

(Press Enter to move on whenever you're ready.)

```
(You keep walking. The cave has nothing new to say.)
```

### 12. Which way? _(visit 2)_

A faded sign reads: NORTH — passages, SOUTH — meadow, EAST — see manager.

**Inputs:**

- `direction` = `north`

→ jumped to `cave-fork`

### 13. The forked passage _(visit 2)_

The northern passage forks. The left tunnel is dark and silent.
The right tunnel has cool air and faint scratching noises.

**Inputs:**

- `fork` = `left`

→ jumped to `dark-passage`

### 14. The dark passage _(visit 2)_

You stumble through the dark. Your hand finds wet stone. Somewhere
ahead, something that is definitely not a small mouse is breathing.

**Inputs:**

- `nerve` = `forward`

→ jumped to `dragon`

### 15. The dragon's lair _(visit 2)_

A red dragon reclines on a heap of gold, flipping through a magazine
titled *Hoard Quarterly*. It glances up at you.

"Oh good. Lunch."

(The reveal streams line-by-line — press Enter at any point to
skip ahead to the verdict.)

```
[38;5;245m      The cave widens. Your torch flickers and gives up entirely.[0m
[38;5;245m      Something else takes over the lighting.[0m

[36m                    /\        /\        /\        /\[0m
[36m                   /  \      /  \      /  \      /  \[0m
[36m                  /    \    /    \    /    \    /    \[0m
[36m                 /______\  /______\  /______\  /______\[0m

[38;5;88m                              \||/[0m
[38;5;88m                              |  [1;33m@[38;5;88m___oo[0m
[38;5;88m                    /\  /\   / (__,,,,|[0m
[38;5;88m                   ) /^\) ^\/ _)[0m
[38;5;88m                   )   /^\/   _)[0m
[38;5;88m                   )   _ /  / _)[0m
[38;5;88m                  /\  )/\/ ||  | )_)[0m
[38;5;88m                 <  >      |(,,) )__)[0m
[38;5;88m                  ||      /    \)___)\[0m
[38;5;88m                  | \____(      )___) )___[0m
[38;5;88m                   \______(_______;;; __;;;[0m

[38;5;220m      Around him: rivers of gold, sparkling like dirty fountains.[0m
[2m      A magazine titled "[0m[1mHoard Quarterly[0m[2m" rests across one claw.[0m
[2m      He licks a page with a forked tongue and turns it.[0m
[2m      The article is a six-page spread on adamantine polishing.[0m

[1;33m      Slowly, two enormous yellow eyes lift to meet yours.[0m

[1;31m      "Oh good,"[0m[31m he says, conversationally. [1;31m"Lunch."[0m
```

→ jumped to `death`

### 16. You died (sort of) _(visit 2)_

The dragon yawns, exhales, and you become a statistic.

Some time passes. You wake up at the cave entrance with no memory of
dying. Your boots crunch on something that might have been a previous
adventurer.

→ jumped to `entrance`

### 17. The entrance _(visit 3)_

The cave mouth yawns dark. A breeze stirs the tall grass behind you.
Three paths lead onward. Your boots crunch on something that might
have been a previous adventurer.

### 18. A formal introduction _(visit 3)_

The cave breeze coils around you. It seems to want a formal greeting
before letting you any further in.

**Inputs:**

- `name` = `Wanderer`

```
(The cave murmurs: "Back so soon, Wanderer the Foolhardy?")
```

### 19. Listen for a moment _(visit 3)_

The cave is full of small sounds. If you stand still and listen,
you might catch a hint of what waits inside.

(Press Enter to move on whenever you're ready.)

```
(You keep walking. The cave has nothing new to say.)
```

### 20. Which way? _(visit 3)_

A faded sign reads: NORTH — passages, SOUTH — meadow, EAST — see manager.

**Inputs:**

- `direction` = `north`

→ jumped to `cave-fork`

### 21. The forked passage _(visit 3)_

The northern passage forks. The left tunnel is dark and silent.
The right tunnel has cool air and faint scratching noises.

**Inputs:**

- `fork` = `left`

→ jumped to `dark-passage`

### 22. The dark passage _(visit 3)_

You stumble through the dark. Your hand finds wet stone. Somewhere
ahead, something that is definitely not a small mouse is breathing.

**Inputs:**

- `nerve` = `forward`

→ jumped to `dragon`

### 23. The dragon's lair _(visit 3)_

A red dragon reclines on a heap of gold, flipping through a magazine
titled *Hoard Quarterly*. It glances up at you.

"Oh good. Lunch."

(The reveal streams line-by-line — press Enter at any point to
skip ahead to the verdict.)

```
[38;5;245m      The cave widens. Your torch flickers and gives up entirely.[0m
[38;5;245m      Something else takes over the lighting.[0m

[36m                    /\        /\        /\        /\[0m
[36m                   /  \      /  \      /  \      /  \[0m
[36m                  /    \    /    \    /    \    /    \[0m
[36m                 /______\  /______\  /______\  /______\[0m

[38;5;88m                              \||/[0m
[38;5;88m                              |  [1;33m@[38;5;88m___oo[0m
[38;5;88m                    /\  /\   / (__,,,,|[0m
[38;5;88m                   ) /^\) ^\/ _)[0m
[38;5;88m                   )   /^\/   _)[0m
[38;5;88m                   )   _ /  / _)[0m
[38;5;88m                  /\  )/\/ ||  | )_)[0m
[38;5;88m                 <  >      |(,,) )__)[0m
[38;5;88m                  ||      /    \)___)\[0m
[38;5;88m                  | \____(      )___) )___[0m
[38;5;88m                   \______(_______;;; __;;;[0m

[38;5;220m      Around him: rivers of gold, sparkling like dirty fountains.[0m
[2m      A magazine titled "[0m[1mHoard Quarterly[0m[2m" rests across one claw.[0m
[2m      He licks a page with a forked tongue and turns it.[0m
[2m      The article is a six-page spread on adamantine polishing.[0m

[1;33m      Slowly, two enormous yellow eyes lift to meet yours.[0m

[1;31m      "Oh good,"[0m[31m he says, conversationally. [1;31m"Lunch."[0m
```

→ jumped to `death`

### 24. You died (sort of) _(visit 3)_

The dragon yawns, exhales, and you become a statistic.

Some time passes. You wake up at the cave entrance with no memory of
dying. Your boots crunch on something that might have been a previous
adventurer.

→ jumped to `entrance`

### 25. The entrance _(visit 4)_

The cave mouth yawns dark. A breeze stirs the tall grass behind you.
Three paths lead onward. Your boots crunch on something that might
have been a previous adventurer.

> **Error:** max visits (3) exceeded for step "entrance"

