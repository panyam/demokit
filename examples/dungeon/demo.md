---
title: Cave of Cards
description: A small directed-graph adventure with cycles, convergence, and one piece of state
actors:
  - { id: You,    label: You }
  - { id: Cave,   label: The Cave }
  - { id: Dragon, label: Dragon }
---

## Welcome, adventurer {#welcome}

You came for the treasure. Or the glory. Or because the rent is due
and the dragon hoard is famously well-funded.

Make your choices. Try not to die. If you do die, the cave is
suspiciously generous about second chances.

## The entrance {#entrance}

> The cave mouth yawns dark. A breeze stirs the tall grass behind you.
> Three paths lead onward. Your boots crunch on something that might
> have been a previous adventurer.

## A formal introduction {#name}

> The cave breeze coils around you. It seems to want a formal greeting
> before letting you any further in.

```inputs
- name: name
  prompt: Your name, adventurer
  type: string
  default: Wanderer
```

## Listen for a moment {#listen}

> The cave is full of small sounds. If you stand still and listen,
> you might catch a hint of what waits inside.
>
> (Press Enter to move on whenever you're ready.)

## Which way? {#choose-dir}

> A faded sign reads: NORTH — passages, SOUTH — meadow, EAST — see manager.

```inputs
- name: direction
  prompt: Which way? (north/south/east)
  type: choice
  options: [north, south, east]
  default: north
```

## The sunny meadow {#meadow}

> Wildflowers nod in the breeze. A stone fountain bubbles in the center.
> Something glints under the water — copper, probably. Or a ring. Or a
> particularly shiny piece of trash.
>
> A small brass plaque reads: "Make a wish. Tribute appreciated."

```inputs
- name: tribute
  prompt: How many coins do you toss in? (0 = walk away)
  type: int
  default: 1
```

## The sparkling pool {#ring-cave}

> Up close, the fountain isn't a fountain at all — it's a pool, glowing
> with a faint green light. At the bottom: a small silver ring with a
> stone the color of twilight.
>
> You pocket it. The cave feels less hostile somehow.

## The forked passage {#cave-fork}

> The northern passage forks. The left tunnel is dark and silent.
> The right tunnel has cool air and faint scratching noises.

```inputs
- name: fork
  prompt: Which tunnel? (left/right)
  type: choice
  options: [left, right]
  default: left
```

## The dark passage {#dark-passage}

> You stumble through the dark. Your hand finds wet stone. Somewhere
> ahead, something that is definitely not a small mouse is breathing.

```inputs
- name: nerve
  prompt: Press on or retreat? (forward/back)
  type: choice
  options: [forward, back]
  default: forward
```

## The goblin den {#goblin}

> A goblin sits on an overturned bucket, polishing a curved knife and
> humming. He looks up, smiles a smile with one too many teeth.
>
> "Oh good," he says. "A volunteer."

```inputs
- name: response
  prompt: Fight or flee? (fight/flee)
  type: choice
  options: [fight, flee]
  default: flee
```

```refs
- name: Goblin Hospitality Code §4
  url: https://example.invalid/ghc-4
```

## The dragon's lair {#dragon}

> A red dragon reclines on a heap of gold, flipping through a magazine
> titled *Hoard Quarterly*. It glances up at you.
>
> "Oh good. Lunch."

## Victorious! {#victory}

> The silver ring blazes with white light. The dragon squints, shrugs,
> and returns to its magazine.
>
> "Take what you can carry," it says. "I'm overdue for a declutter."
>
> You stuff your pockets with gold and a few interesting back issues.

## You died (sort of) {#death}

> The dragon yawns, exhales, and you become a statistic.
>
> Some time passes. You wake up at the cave entrance with no memory of
> dying. Your boots crunch on something that might have been a previous
> adventurer.

## Another go? {#again}

> The sunlight is warm. The cave is patient.

```inputs
- name: again
  prompt: Try again? (yes/no)
  type: choice
  options: [yes, no]
  default: no
```

## Farewell {#end}

You blink in the sunlight. The cave hums quietly to itself.

Whether it was all a dream is, like most things, optional.
