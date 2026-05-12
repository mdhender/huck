# Miko's design notes for admin tasks

> These are not really workflows. They’re **administrative chores**. That’s an important distinction.
>
> I would design them as boring, safe, predictable admin screens. No cleverness.

## Admin area shape

I’d group these into three sidebar sections:

**Administration**

* Users
* Invitations
* Games
* Server

Not too many levels. The admin does not need a maze.

## 1. User management

This should feel like an address book plus account status.

Main screen: **Users table**

Columns:

* Name
* Email
* Role
* Active status
* Last login
* Created
* Actions

Actions:

* View/Edit
* Reset password
* Deactivate / Reactivate

For soft bans, avoid scary language in the UI. Use:

> Active / Suspended

Not “banned” unless you want the emotional weight.

## 1a. Invitations

Separate screen, not hidden inside Users.

Columns:

* Email
* Role
* Status: Pending / Accepted / Expired / Revoked
* Sent date
* Expires date
* Actions

Actions:

* Create invitation
* Resend
* Revoke
* Copy invite link

This should be calm and auditable.

## 1b. Password resets

For admins, this should be a task on the user detail page:

> Send password reset email

Then confirmation:

> Password reset email sent to [jane@example.com](mailto:jane@example.com).

I would not let admins set passwords manually unless absolutely necessary.

## 1c. Active status

On the user detail page, make this very clear:

> Account status: Active

Button:

> Suspend account

Require confirmation, but not a dramatic modal unless needed.

Suspended users should remain visible in the user list. Never hide administrative history.

## 2. Game management

Main screen: **Games table**

Columns:

* Game name
* Status
* Current turn
* Players
* Gamemasters
* Created
* Last activity
* Actions

Statuses:

* Setup
* Active
* Paused
* Archived

Actions:

* View/Edit
* Manage gamemasters
* Archive

## 2a. Create new game

This can be a simple form:

* Name
* Short code / slug
* Description
* Initial status
* Starting turn number
* Assigned gamemasters

Do not overbuild this at first.

## 2b. Add/delete gamemasters

I’d handle this inside the game detail page as a dedicated panel:

**Gamemasters**

* Current gamemasters list
* Add gamemaster
* Remove

Removing the last gamemaster should probably be blocked or require elevated admin action.

## 2c. Archive

Archive should mean:

> Remove from active operational screens, preserve data.

Use careful language:

> Archive game
> This will hide the game from active dashboards but preserve its records and artifacts.

Possibly require typing the game name only if archive has serious consequences. Otherwise a normal confirmation is enough.

## 3. Server management

This is different. This is not CRUD. This is an operations console.

I would make this a **Server** page with two sections:

### Server Status

Dashboard-style facts:

* Huck version
* database schema version
* database size
* active users
* running services
* uptime
* environment
* last backup, if available

### Server Actions

Dangerous tasks:

* Stop services
* Restart services, if supported
* Maintenance mode, if supported

Dangerous actions should be visually separated from normal information.

Use language like:

> Operational Actions

Not buried next to harmless status cards.

## Stop services

This needs friction.

Not silly friction, but real intentionality.

Suggested pattern:

Button:

> Stop services

Confirmation screen:

> Stop Huck services?

Explain:

> Active users may be disconnected. Uploads or turn processing may be interrupted.

Require:

* confirm checkbox, or
* type `STOP`

Then button:

> Stop services

Destructive buttons should never be the first button on the page.

## Miko’s recommendation

Do not make admin beautiful.

Make it:

* legible,
* auditable,
* boring,
* hard to misuse,
* easy to recover from.

For Huck admin, “boring” is a compliment.

