# Huck Front-End Vision & Design Philosophy

## Purpose

Huck is a platform for old-school play-by-mail and persistent strategy games that have been modernized to run behind a browser.

Huck is not:
- a social platform,
- a game discovery portal,
- a modern SaaS dashboard,
- or an “engagement” product.

Huck exists to provide:
- authentication,
- authorization,
- account management,
- uploads/downloads,
- notifications,
- operational workflows,
- and a trusted browser-based interface for PBM-style games.

Game developers are expected to replace the demo game engine with their own implementations.

Huck provides the platform shell and operational infrastructure.

---

# Core Emotional Goal

The interface should feel:

- calm,
- patient,
- trustworthy,
- unsurprising,
- durable,
- readable,
- familiar.

A successful emotional reaction from users would be:

> “This fits me like an old cardigan.”

Users should feel that:
- the software respects them,
- the software is stable,
- the software is understandable,
- and the software is not trying to manipulate them.

---

# Target Audience

## Primary Audience (80%)

Older PBM and strategy gamers who have been playing since the 1980s.

Characteristics:
- comfortable with desktop applications,
- comfortable with spreadsheets,
- often familiar with Excel or Lotus-era workflows,
- not highly technical,
- focused on gameplay rather than social interaction.

These users:
- prefer desktop and laptop workflows,
- tolerate information density,
- value clarity over visual flair,
- strongly dislike ambiguity in navigation and workflows.

## Secondary Audience (Growing)

Younger retro-gaming enthusiasts.

Characteristics:
- technically sophisticated,
- nostalgic for older interfaces,
- primarily use laptops/desktops for gameplay,
- use mobile devices mainly for status checks and notifications.

These users are attracted to:
- clean operational interfaces,
- retro structural aesthetics,
- and trustworthy software.

## Gamemasters

Gamemasters are operational users.

They are accustomed to:
- CRUD-heavy database applications,
- administrative tools,
- dense forms and tables,
- hierarchical workflows.

Gamemasters strongly value:
- breadcrumbs,
- stable layouts,
- dense information displays,
- and operational clarity.

---

# Core Product Philosophy

Huck is operational software.

It is:
- transactional,
- procedural,
- workflow-oriented,
- trust-oriented.

Players are not spending hours socially interacting with the site.

Typical player workflow:
1. Log in.
2. Check game status.
3. Download turn artifacts.
4. Upload orders.
5. Resolve validation issues.
6. Submit orders.
7. Leave the site.

The software should minimize friction in this workflow.

---

# Design Principles

## 1. No Mystery

Users should always know:
- where they are,
- what state the system is in,
- what actions are available,
- and what happens next.

Avoid:
- hidden actions,
- icon-only controls,
- hover-only interactions,
- disappearing UI,
- unexplained state changes.

---

## 2. Readability Is Beauty

Readable interfaces are more important than dramatic aesthetics.

Prioritize:
- line length,
- spacing,
- typography,
- contrast,
- stable layouts,
- and clear hierarchy.

Avoid:
- excessively wide text blocks,
- cramped layouts,
- tiny text,
- forced dark mode,
- and unnecessary visual noise.

---

## 3. Desktop First

Huck is designed primarily for desktop and laptop users.

Mobile support is important for:
- status checks,
- notifications,
- and lightweight interactions.

However:
- order entry,
- administration,
- and detailed workflows
are desktop-oriented experiences.

Navigation and layout decisions should prioritize desktop workflows first.

---

## 4. Calm Interfaces

Huck should feel like:
- office software,
- operational software,
- or a beloved desktop application.

Not:
- a startup dashboard,
- a social feed,
- or a game launcher.

The interface should not compete for attention.

Avoid:
- excessive animation,
- aggressive notifications,
- flashy transitions,
- or “engagement” mechanics.

---

## 5. Stable Navigation

The interface should remain structurally stable.

Preferred navigation:
- persistent left sidebar,
- visible navigation labels,
- breadcrumbs,
- consistent page structure.

Avoid:
- hamburger-first navigation,
- layout shifts,
- and collapsing core navigation.

Users should not need to “hunt” for functionality.

---

# Structural Recommendations

## Persistent Sidebar

Use a permanent left sidebar on desktop.

Player examples:
- Home
- My Games
- Current Turn
- Downloads
- Upload Orders
- Account

Gamemaster examples:
- Games
- Players
- Turn Processing
- Validation Errors
- Artifacts
- Settings

The sidebar should prioritize:
- predictability,
- stability,
- and clarity.

---

## Breadcrumbs

Breadcrumbs are strongly encouraged.

Huck has real hierarchy:
- Game
- Turn
- Artifact
- Orders
- Players
- Validation

Breadcrumbs reinforce:
- orientation,
- confidence,
- and recoverability.

Gamemasters strongly prefer them.

---

## Status Cards

Player home screens should use compact operational status cards.

Cards should answer:

> “What do I need to do next?”

Typical card contents:
- game name,
- current turn,
- deadlines,
- artifact availability,
- order submission status,
- validation state,
- last updated timestamp.

Cards should emphasize action and status, not decoration.

---

## Wizard-Based Workflows

Uploads and submissions should use explicit multi-step workflows.

Preferred flow:
1. Upload Orders
2. Validate
3. Review Errors
4. Submit
5. Confirmation

This reduces:
- anxiety,
- ambiguity,
- and accidental submission mistakes.

Users should always know:
- their current step,
- completed steps,
- and next actions.

---

# Visual Direction

## Desired Aesthetic

Huck should resemble:
- a well-loved desktop database application,
- calm office software,
- a trusted operational console,
- or classic administrative software updated with good web manners.

Possible inspirations:
- desktop business software,
- older database front ends,
- operational dashboards,
- restrained strategy game tooling.

---

## Avoid

Avoid:
- cyberpunk terminal aesthetics,
- gamer RGB styling,
- modern startup SaaS aesthetics,
- excessive card layouts,
- mobile-first emptiness,
- forced dark themes,
- “Discord-like” interfaces,
- and novelty UI experiments.

---

# Typography & Layout

Typography matters more than illustration.

Priorities:
- strong readability,
- slightly larger default text,
- restrained use of color,
- consistent spacing,
- and high information clarity.

Use:
- controlled line widths,
- multi-column layouts on widescreens,
- stable scrolling behavior,
- and predictable forms.

Avoid:
- giant empty whitespace,
- excessively narrow mobile-style layouts on desktop,
- and janky scrolling behavior.

---

# Density Strategy

Players and gamemasters require different densities.

## Player Screens
- calmer,
- more spacious,
- focused on clarity and workflows.

## Gamemaster Screens
- denser,
- more table-oriented,
- optimized for operational efficiency.

Both should share:
- the same typography,
- navigation patterns,
- and overall visual language.

---

# Current Technology Direction

Pico.css is currently a strong fit for Huck.

Advantages:
- semantic HTML,
- readable defaults,
- accessibility-friendly structure,
- low friction,
- easy progressive enhancement,
- desktop-friendly behavior.

However, Huck should gradually evolve:
- its own spacing system,
- navigation shell,
- table standards,
- form standards,
- breadcrumb styling,
- and workflow components.

Pico should become:
- the baseline foundation,
not the entire design system.

---

# Non-Negotiable Requirements

## Trust & Privacy

The worst possible outcome is exposing player data to other players.

Trust is foundational.

Operational workflows must communicate:
- success,
- failure,
- and validation state clearly.

---

## Upload Reliability

The upload workflow is sacred.

Uploading orders must:
- feel reliable,
- provide immediate feedback,
- and clearly communicate success or failure.

Everything else is secondary.

---

# Final Philosophy

Huck should not feel trendy.

It should feel:
- dependable,
- understandable,
- humane,
- and durable.

The ideal user reaction is:

> “I know how this works.”
>
> “I trust this system.”
>
> “I can focus on the game.”
