// Marks a control whose current selection is a resolved default. Fields
// preselect the real value they will use rather than a "default" placeholder the
// user would have to remember, so the caption is what tells them it is inherited.
export function FieldDefaultHint({ text }: { text: string }) {
	return <span className="wd-caption shrink-0 text-wd-tertiary">{text}</span>;
}
