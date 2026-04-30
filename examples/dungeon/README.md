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
(The cave murmurs: "Welcome, Wanderer the Surprisingly Tall.")
```

### 3. Which way?

A faded sign reads: NORTH — passages, SOUTH — meadow, EAST — see manager.

**Inputs:**

- `direction` = `north`

→ jumped to `cave-fork`

### 4. The forked passage

The northern passage forks. The left tunnel is dark and silent.
The right tunnel has cool air and faint scratching noises.

**Inputs:**

- `fork` = `left`

→ jumped to `dark-passage`

### 5. The dark passage

You stumble through the dark. Your hand finds wet stone. Somewhere
ahead, something that is definitely not a small mouse is breathing.

**Inputs:**

- `nerve` = `forward`

→ jumped to `dragon`

### 6. The dragon's lair

A red dragon reclines on a heap of gold, flipping through a magazine
titled *Hoard Quarterly*. It glances up at you.

"Oh good. Lunch."

→ jumped to `death`

### 7. You died (sort of)

The dragon yawns, exhales, and you become a statistic.

Some time passes. You wake up at the cave entrance with no memory of
dying. Your boots crunch on something that might have been a previous
adventurer.

→ jumped to `entrance`

### 8. The entrance _(visit 2)_

The cave mouth yawns dark. A breeze stirs the tall grass behind you.
Three paths lead onward. Your boots crunch on something that might
have been a previous adventurer.

### 9. A formal introduction _(visit 2)_

The cave breeze coils around you. It seems to want a formal greeting
before letting you any further in.

**Inputs:**

- `name` = `Wanderer`

```
(The cave murmurs: "Back so soon, Wanderer the Surprisingly Tall?")
```

### 10. Which way? _(visit 2)_

A faded sign reads: NORTH — passages, SOUTH — meadow, EAST — see manager.

**Inputs:**

- `direction` = `north`

→ jumped to `cave-fork`

### 11. The forked passage _(visit 2)_

The northern passage forks. The left tunnel is dark and silent.
The right tunnel has cool air and faint scratching noises.

**Inputs:**

- `fork` = `left`

→ jumped to `dark-passage`

### 12. The dark passage _(visit 2)_

You stumble through the dark. Your hand finds wet stone. Somewhere
ahead, something that is definitely not a small mouse is breathing.

**Inputs:**

- `nerve` = `forward`

→ jumped to `dragon`

### 13. The dragon's lair _(visit 2)_

A red dragon reclines on a heap of gold, flipping through a magazine
titled *Hoard Quarterly*. It glances up at you.

"Oh good. Lunch."

→ jumped to `death`

### 14. You died (sort of) _(visit 2)_

The dragon yawns, exhales, and you become a statistic.

Some time passes. You wake up at the cave entrance with no memory of
dying. Your boots crunch on something that might have been a previous
adventurer.

→ jumped to `entrance`

### 15. The entrance _(visit 3)_

The cave mouth yawns dark. A breeze stirs the tall grass behind you.
Three paths lead onward. Your boots crunch on something that might
have been a previous adventurer.

### 16. A formal introduction _(visit 3)_

The cave breeze coils around you. It seems to want a formal greeting
before letting you any further in.

**Inputs:**

- `name` = `Wanderer`

```
(The cave murmurs: "Back so soon, Wanderer the Surprisingly Tall?")
```

### 17. Which way? _(visit 3)_

A faded sign reads: NORTH — passages, SOUTH — meadow, EAST — see manager.

**Inputs:**

- `direction` = `north`

→ jumped to `cave-fork`

### 18. The forked passage _(visit 3)_

The northern passage forks. The left tunnel is dark and silent.
The right tunnel has cool air and faint scratching noises.

**Inputs:**

- `fork` = `left`

→ jumped to `dark-passage`

### 19. The dark passage _(visit 3)_

You stumble through the dark. Your hand finds wet stone. Somewhere
ahead, something that is definitely not a small mouse is breathing.

**Inputs:**

- `nerve` = `forward`

→ jumped to `dragon`

### 20. The dragon's lair _(visit 3)_

A red dragon reclines on a heap of gold, flipping through a magazine
titled *Hoard Quarterly*. It glances up at you.

"Oh good. Lunch."

→ jumped to `death`

### 21. You died (sort of) _(visit 3)_

The dragon yawns, exhales, and you become a statistic.

Some time passes. You wake up at the cave entrance with no memory of
dying. Your boots crunch on something that might have been a previous
adventurer.

→ jumped to `entrance`

