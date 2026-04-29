# Office Coffee Crisis

A short choose-your-own-adventure about the morning caffeine ritual

## What you'll learn

- **Approach the machine** — The machine has three buttons. None of them are labelled in any way that inspires confidence.
- **Pick a button** — Black is reliable. Sugar is suspicious. Wild Card is, allegedly, what Karen drinks.
- **Black coffee** — Dependable. Functional. Tastes like burnt cardboard.
- **Sugar overload** — Tastes great for 90 seconds. The crash will be biblical.
- **Transformed** — You are, briefly, a cat. Karen does not seem surprised.

## Flow

```mermaid
sequenceDiagram
    participant You
    participant Machine as Coffee Machine
    participant Karen as Karen from Accounting

    Note over You,Karen: Step 1: Approach the machine
    You->>Machine: shuffle forward
    Machine-->>You: [hum of disappointment]

    Note over You,Karen: Step 2: Pick a button

    Note over You,Karen: Step 3: Black coffee
    Machine->>You: pours scalding hot bitter liquid

    Note over You,Karen: Step 4: Sugar overload
    Machine->>You: syrupy sludge with whipped foam

    Note over You,Karen: Step 5: Wild card
    Machine->>You: ??? (smells faintly of cilantro)

    Note over You,Karen: Step 6: Drink the wild card?
    Karen->>You: raises an eyebrow from across the kitchen

    Note over You,Karen: Step 7: Transformed

    Note over You,Karen: Step 8: Dignified retreat

    Note over You,Karen: Step 9: Try a different button?

    Note over You,Karen: Step 10: End
```

## Steps

### Setting the scene

It is 9:01 AM. You did not sleep enough. The standup is in 14 minutes.
You approach the office coffee machine with the focus of a samurai.

### Step 1: Approach the machine

The machine has three buttons. None of them are labelled in any way that inspires confidence.

### Step 2: Pick a button

Black is reliable. Sugar is suspicious. Wild Card is, allegedly, what Karen drinks.

### Step 3: Black coffee

Dependable. Functional. Tastes like burnt cardboard.

### Step 4: Sugar overload

Tastes great for 90 seconds. The crash will be biblical.

### Step 5: Wild card

### Step 6: Drink the wild card?

### Step 7: Transformed

You are, briefly, a cat. Karen does not seem surprised.

### Step 8: Dignified retreat

### Step 9: Try a different button?

### Step 10: End

## Run it

```bash
go run ./examples/basic/
```

Pass `--non-interactive` to skip pauses:

```bash
go run ./examples/basic/ --non-interactive
```
