# Admin UI Conventions

This UI is an operator console. Keep pages dense, predictable, and aligned with
the existing shadcn components in `cmd/ui/components/ui`.

## Page Structure

- Use `PageHeader` for breadcrumbs and page-level actions.
- Use `PageSummaryCard` for identity, description, and compact metadata only.
- Do not put page-level actions such as New, Reload, or Delete inside summary
  cards or content card headers.
- Detail pages should put Back, Reload, and destructive page actions in
  `PageHeader.actions`.

## Header Actions

- Use compact buttons for header actions:
  `className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm"`.
- Use `variant="outline"` for normal header actions, including New and Refresh.
- Use `DeleteConfirmButton` for destructive header actions.
- Use lucide icons in buttons when an obvious icon exists.

## List Pages

- List pages should use two main cards: a top `PageSummaryCard` for page info
  and metadata, then one table `Card` for the list.
- List pages that expose multiple related list tables may use `Tabs` to switch
  between those tables.
- List pages should show create actions in `PageHeader.actions`, not as a form
  card above the table.
- For tabbed list pages, all list-level create actions still belong in
  `PageHeader.actions`, not inside each tab's table card header.
- Create flows should open a `Dialog` with `DialogTitle` and
  `DialogDescription`.
- Every resource list should display the resource id or primary stable id.
- The first table column should be the unique list id for that row. If an API
  identifies a row by a compound path, show and copy the full compound id.
- Resource ids should be copyable from the list row.
- Long or compound ids must stay width-constrained in the table; keep the full
  value available through copy and tooltip instead of wrapping the row tall.
- List tables should fit the default desktop content width without horizontal
  scrolling. Treat default horizontal scroll as a review finding; reduce fixed
  widths, use `table-fixed`, and truncate long values before accepting it.
- Copy buttons should follow the peer list pattern: show the copy icon by
  default, then replace it with a check icon after a successful copy.
- In a multi-line ID cell, keep the copy button outside the text block and
  vertically centered against the full ID block, not attached to the first line.
- Table rows should open the primary detail page by clicking the row or the
  resource id/name.
- Do not add an `Open` button to each row when row navigation is available.
- Do not add row action columns to list tables. Destructive and resource-level
  operations belong on the detail page.
- The final table column should be `Updated`, not `Actions`.
- Set explicit widths for fixed-purpose columns, use `table-fixed` when a table
  has long ids, and truncate long values instead of letting columns expand.
- Buttons inside clickable rows must stop propagation so they do not also open
  the row target.

## Detail Pages

- Detail page summary cards should not contain action buttons.
- Use tabs for distinct resource sub-surfaces such as Info, Members, Invite
  Token, and History.
- Edit forms belong in the relevant detail tab or a dialog, not in the summary
  card.

## Forms

- Use existing form helpers such as `FormField` and shadcn inputs.
- Dialog create forms should close only after the create operation succeeds or
  resolves to an existing resource.
- Keep sensitive or destructive operations behind confirmation dialogs.
