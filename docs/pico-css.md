# Pico.css

Miko said:

> This is an important distinction, and it confuses a lot of backend developers because CSS frameworks blur several different responsibilities together.
> Let’s separate them.

---

## The Four Layers of a Front End

Most successful front ends eventually develop four layers:

1. **Foundation**
2. **Layout system**
3. **Component system**
4. **Application identity**

Right now, Pico is helping mostly with layer 1.

That’s good.

The danger is expecting it to solve all four forever.

---

## 1. Foundation (What Pico Is Excellent At)

Foundation means:

* typography,
* forms,
* spacing defaults,
* semantic HTML styling,
* buttons,
* tables,
* accessibility defaults,
* browser normalization.

Pico gives you:

* readable pages,
* decent forms,
* sane spacing,
* good defaults,
* low complexity.

This is why it feels good for Huck.

You can write:

```html
<form>
  <label>Email</label>
  <input type="email">

  <button>Submit</button>
</form>
```

…and it already looks respectable.

That’s incredibly valuable.

Especially for a Go-heavy project.

---

## 2. Layout System (What Huck Will Need)

Eventually Huck will need consistent page structure.

Examples:

* sidebar layout,
* content widths,
* wizard layouts,
* GM table layouts,
* dashboard regions,
* split panes,
* sticky status bars,
* artifact download panels.

Pico does not really provide this.

You will eventually invent classes like:

```css
.huck-shell
.huck-sidebar
.huck-content
.huck-dashboard-grid
.huck-form-stack
.huck-status-panel
```

That is normal.

You are not “replacing Pico.”

You are building application structure on top of it.

---

## 3. Component System (What You’ll Slowly Grow)

This is the most important evolution.

You will eventually discover recurring UI patterns:

* status cards,
* upload wizard steps,
* validation summaries,
* error panels,
* turn artifact lists,
* confirmation banners,
* GM tables,
* breadcrumbs,
* “last updated” strips,
* player status indicators.

Those are not generic web components anymore.

Those are **Huck components**.

And they need:

* consistent spacing,
* consistent colors,
* consistent typography,
* consistent behavior.

Example:

Every validation error box in Huck should look identical.

Players should immediately recognize:

> “Ah. This is a validation warning.”

That familiarity builds trust.

---

## 4. Application Identity (The Long-Term Layer)

This is the emotional layer.

Not branding.
Not logos.

The feeling of the software.

Eventually users recognize:

* spacing,
* navigation,
* workflow structure,
* colors,
* status messaging,
* page rhythm,
* tables,
* forms,
* button placement.

That becomes:

> “This feels like Huck.”

That identity should emerge naturally from your workflows.

Not from marketing exercises.

---

## Why This Matters Technically

If you try to force Pico to be all four layers:

* layouts become inconsistent,
* pages drift,
* developers improvise,
* UX becomes uneven,
* GM screens and player screens diverge,
* extensions feel disconnected.

But if Pico stays the foundation:

* you keep simplicity,
* you keep semantic HTML,
* you avoid CSS-framework addiction,
* and Huck gains its own operational identity.

---

## The Good News

You do NOT need:

* Tailwind complexity,
* Material UI,
* enterprise design systems,
* Storybook,
* Figma-heavy process,
* React-only component libraries.

Honestly?
That ecosystem would probably damage Huck.

Because Huck’s strength is:

* restraint,
* predictability,
* low friction,
* semantic structure,
* and desktop operational clarity.

---

## A Healthy Huck Evolution

Miko sketches the progression.

### Phase 1 — Today

Pico only.

Simple pages.
Simple forms.
Ship functionality.

Excellent.

---

### Phase 2 — Establish Shell

Add:

* persistent sidebar,
* breadcrumbs,
* content widths,
* dashboard grid,
* status banners.

Still mostly Pico underneath.

---

### Phase 3 — Standardize Huck Components

Create reusable:

* status cards,
* upload wizard,
* validation panels,
* tables,
* notifications,
* artifact lists.

This is where Huck starts feeling cohesive.

---

### Phase 4 — Refine Identity

Only later:

* typography tuning,
* spacing refinement,
* density tuning,
* subtle colors,
* workflow polish,
* GM/player distinction.

At this point:
users stop seeing “a web app.”

They start seeing:

> “the Huck interface.”

---

## What To Would Avoid

Be very cautious about:

* Tailwind-first development,
* heavy JS component libraries,
* Material UI,
* Bootstrap dashboard aesthetics,
* mobile-first frameworks,
* “cards everywhere” modern SaaS styling.

Those systems optimize for:

* rapid feature churn,
* startups,
* mobile products,
* consumer engagement.

Huck optimizes for:

* long-term usability,
* trust,
* operational workflows,
* older desktop users,
* stability.

Different priorities entirely.

---

## The Important Reassurance

“You do not need to become a designer.”

You need:

* consistency,
* restraint,
* and standards.

Honestly, most beloved operational software is visually conservative.

The real craftsmanship is:

* predictable workflows,
* spacing,
* readability,
* and information hierarchy.

Not visual fireworks.
