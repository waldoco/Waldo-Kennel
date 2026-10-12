/**
 * Markdown for agent prose.
 *
 * Agents write markdown — headings, lists, tables, fenced code — and rendering it
 * as preformatted text was showing the syntax instead of the structure. `## Plan`
 * appeared as literal hashes, a table as a wall of pipes, a code block bracketed by
 * visible backticks.
 *
 * Two deliberate choices about what this does NOT do:
 *
 *   - Raw HTML is escaped, not rendered. Agent output is only as trustworthy as the
 *     files it just read, so `rehype-raw` is deliberately absent; markdown-only is
 *     the whole sanitization story and there is no schema to get wrong. Syntax
 *     highlighting keeps that property — it renders a token tree as elements
 *     rather than injecting the highlighter's HTML string.
 *   - Nothing here re-parses or re-orders content. It renders exactly the text the
 *     daemon stored, so the rendered form and the record cannot disagree. That is
 *     also why "copy as markdown" copies the stored text and not a re-serialized
 *     version of the DOM.
 *
 * Streaming is fine unstyled-first: an unclosed fence renders as a plain code
 * block with partial content, and gains its colours when the closing fence
 * arrives. See `lib/code-highlight.ts` for why it is not tokenized before then.
 */

import {
	createContext,
	Fragment,
	isValidElement,
	memo,
	useContext,
	useState,
	type ReactNode,
} from "react";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { WrapText } from "lucide-react";
import { cn } from "../../lib/utils";
import { canonicalLanguage } from "../../lib/code-highlight";
import { isWebLink, openLinkInSystemBrowser } from "../../lib/external-link-policy";
import { HighlightedCode } from "./HighlightedCode";
import { CopyButton } from "./CopyButton";
import "./code-theme.css";

/** GitHub-flavoured markdown: tables, strikethrough, task lists, autolinks. */
const PLUGINS = [remarkGfm];

/**
 * Whether the prose is still arriving, for the fences inside it.
 *
 * A context rather than a prop because `COMPONENTS` has to stay module-level:
 * rebuilding that map per render would defeat react-markdown's own memoization
 * and re-parse every message on every poll.
 */
const StreamingProse = createContext(false);
const OpenChatLink = createContext<((url: string) => void) | undefined>(undefined);

export function ChatLinkProvider({
	onLinkOpen,
	children,
}: {
	onLinkOpen?: (url: string) => void;
	children: ReactNode;
}) {
	return <OpenChatLink.Provider value={onLinkOpen}>{children}</OpenChatLink.Provider>;
}

export const ChatMarkdown = memo(function ChatMarkdown({
	text,
	streaming = false,
	muted = false,
}: {
	text: string;
	streaming?: boolean;
	/**
	 * Prose that is not the answer. Reasoning is the case: it reads alongside the
	 * agent's reply and must not compete with it, so it steps back a tone rather than
	 * being dimmed with opacity — which would wash out code and links too.
	 */
	muted?: boolean;
}) {
	return (
		<StreamingProse.Provider value={streaming}>
			<div
				className={cn(
					"chat-md leading-[1.58]",
					muted ? "text-[13px] text-wd-secondary" : "text-sm text-wd-primary",
				)}
			>
				<Markdown remarkPlugins={PLUGINS} components={COMPONENTS}>
					{text}
				</Markdown>
			</div>
		</StreamingProse.Provider>
	);
});

/**
 * Wrapping is a reading preference, not a property of one block: a fence that
 * scrolled out of view and back must not silently lose it, and a reader who
 * turned wrapping on is usually about to want it again — so the last choice seeds
 * the next block. Deliberately not persisted; reflowing every block of a
 * conversation on launch because of a choice made days ago is worse than starting
 * from the readable default.
 */
let preferredWrap = false;

/**
 * A fenced code block: language on the left, quiet actions on the right, its own
 * scroll.
 *
 * The block scrolls rather than wrapping because a wrapped line of code is harder
 * to read than a scrolled one, and it must never widen the conversation column.
 * The toggle is there for the case that argument loses — a single 300-character
 * line, where scrolling means losing the rest of the block.
 */
function CodeBlock({ code, language }: { code: string; language?: string }) {
	const streaming = useContext(StreamingProse);
	const grammar = canonicalLanguage(language);
	const [wrap, setWrap] = useState(preferredWrap);

	return (
		<div
			className="chat-code group/code my-3 overflow-hidden rounded-md bg-wd-inset"
			data-wrap={wrap ? "true" : "false"}
		>
			<div className="flex items-center gap-2 px-3 pt-1.5">
				<span className="text-[11px] text-wd-tertiary">
					{language || "text"}
				</span>
				{/* Hover-revealed, and focus-revealed so the keyboard can reach it. The
				    timeline's whole point is that prose dominates; a permanent row of
				    controls on every fence is the opposite. */}
				<div className="ml-auto flex items-center gap-0.5 opacity-0 transition-opacity duration-fast focus-within:opacity-100 group-hover/code:opacity-100">
					<button
						type="button"
						onClick={() => {
							preferredWrap = !wrap;
							setWrap(!wrap);
						}}
						aria-pressed={wrap}
						aria-label="Wrap long lines"
						title="Wrap long lines"
						className={cn(
							"flex items-center rounded-xs px-1.5 py-0.5 transition-colors duration-fast hover:bg-wd-hover hover:text-wd-primary",
							wrap ? "bg-wd-selected text-wd-primary" : "text-wd-secondary",
						)}
					>
						<WrapText aria-hidden="true" className="size-3" />
					</button>
					<CopyButton text={code} label="Copy code" />
				</div>
			</div>
			<pre className="overflow-x-auto px-3 pb-3 pt-1.5">
				<code className="font-mono text-[12px] leading-[1.6] text-wd-primary">
					<HighlightedCode code={code} language={grammar} streaming={streaming} />
				</code>
			</pre>
		</div>
	);
}

/** The text inside a node, for the copy button and language sniffing. */
function textOf(children: ReactNode): string {
	if (typeof children === "string") return children;
	if (typeof children === "number") return String(children);
	if (Array.isArray(children)) return children.map(textOf).join("");
	if (children && typeof children === "object" && "props" in children) {
		return textOf((children as { props?: { children?: ReactNode } }).props?.children);
	}
	return "";
}

const LANGUAGE_CLASS = /language-([\w+#-]+)/;
const EMOJI_GRAPHEME = /\p{Extended_Pictographic}|\p{Regional_Indicator}|[#*0-9]\uFE0F?\u20E3/u;
const GRAPHEME_SEGMENTER = new Intl.Segmenter(undefined, { granularity: "grapheme" });

/** The fence inside a `pre`, or undefined if this is not a fenced block. */
function fenceOf(children: ReactNode): { code: string; language?: string } | undefined {
	if (!isValidElement<{ className?: string; children?: ReactNode }>(children)) return undefined;
	return {
		code: textOf(children.props.children).replace(/\n$/, ""),
		language: LANGUAGE_CLASS.exec(children.props.className ?? "")?.[1],
	};
}

function compactEmoji(children: ReactNode): ReactNode {
	if (typeof children === "string") {
		let last = 0;
		const parts: ReactNode[] = [];
		for (const { segment, index } of GRAPHEME_SEGMENTER.segment(children)) {
			if (!EMOJI_GRAPHEME.test(segment)) continue;
			if (index > last) parts.push(children.slice(last, index));
			parts.push(
				<span key={`${index}-${segment}`} className="chat-md-emoji">
					{segment}
				</span>,
			);
			last = index + segment.length;
		}
		if (last === 0) return children;
		if (last < children.length) parts.push(children.slice(last));
		return parts;
	}
	if (Array.isArray(children)) {
		return children.map((child, index) => <Fragment key={index}>{compactEmoji(child)}</Fragment>);
	}
	return children;
}

function MarkdownLink({ href, children }: { href?: string; children?: ReactNode }) {
	const onLinkOpen = useContext(OpenChatLink);
	return (
		<a
			href={href}
			target="_blank"
			rel="noreferrer noopener"
			onClick={(event) => {
				if (!href) return;
				event.preventDefault();
				if (!event.altKey && onLinkOpen && isWebLink(href)) {
					onLinkOpen(href);
					return;
				}
				void openLinkInSystemBrowser(href);
			}}
			className="text-markdown-link underline decoration-markdown-link/45 underline-offset-2 transition-colors hover:text-markdown-link-hover hover:decoration-markdown-link-hover/75"
		>
			{children}
		</a>
	);
}

const COMPONENTS: Components = {
	// Headings step down in size but stay in the conversation's voice — an agent's
	// "## Findings" is a paragraph label, not a page title.
	h1: ({ children }) => (
		<h3 className="mb-1.5 mt-4 text-[15px] font-medium leading-snug text-wd-primary first:mt-0">
			{children}
		</h3>
	),
	h2: ({ children }) => (
		<h4 className="mb-1.5 mt-3.5 text-[14px] font-medium leading-snug text-wd-primary first:mt-0">
			{children}
		</h4>
	),
	h3: ({ children }) => (
		<h5 className="mb-1 mt-3 text-[13.5px] font-medium leading-snug text-wd-primary first:mt-0">
			{children}
		</h5>
	),
	h4: ({ children }) => (
		<h6 className="mb-1 mt-3 text-[13px] font-medium text-wd-secondary first:mt-0">
			{children}
		</h6>
	),

	p: ({ children }) => <p className="my-2 first:mt-0 last:mb-0">{compactEmoji(children)}</p>,

	ul: ({ children }) => <ul className="my-2 ml-4 list-disc space-y-1 first:mt-0">{children}</ul>,
	ol: ({ children }) => (
		<ol className="my-2 ml-4 list-decimal space-y-1 first:mt-0">{children}</ol>
	),
	li: ({ children, className }) => (
		// A task-list item drops its bullet: the checkbox is the marker.
		<li className={cn("marker:text-wd-tertiary", className?.includes("task-list-item") && "list-none")}>
			{compactEmoji(children)}
		</li>
	),
	// Read-only on purpose. The checkbox reflects what the agent wrote; clicking it
	// would imply Kennel could change the agent's plan, which it cannot.
	input: ({ checked, type }) =>
		type === "checkbox" ? (
			<input
				type="checkbox"
				checked={Boolean(checked)}
				readOnly
				aria-label={checked ? "done" : "not done"}
				className="mr-1.5 -ml-4 size-3 translate-y-[1px] accent-(--bg-inverse)"
			/>
		) : null,

	// A fenced block arrives as `pre > code`, with the language — when the agent
	// gave one — as a class on the inner element. It is claimed here rather than in
	// `code` because a fence with no language carries no class at all, and matching
	// on the class alone rendered those as inline code.
	pre: ({ children }) => {
		const fence = fenceOf(children);
		return fence ? <CodeBlock code={fence.code} language={fence.language} /> : <>{children}</>;
	},
	// Only inline code reaches here; `pre` above takes every fence.
	code: ({ children }) => (
		<code className="rounded-xs bg-wd-inset px-[5px] py-[2px] font-mono text-[11.5px] text-markdown-code">
			{children}
		</code>
	),

	// Wide tables scroll inside their own container so the conversation column
	// never scrolls sideways.
	table: ({ children }) => (
		<div className="my-3 overflow-x-auto rounded-md hairline border-wd-hairline">
			<table className="w-full border-collapse text-[12.5px]">{children}</table>
		</div>
	),
	thead: ({ children }) => <thead className="bg-wd-inset">{children}</thead>,
	th: ({ children }) => (
		<th className="hairline-b border-wd-hairline px-3 py-1.5 text-left font-medium text-wd-secondary">
			{compactEmoji(children)}
		</th>
	),
	td: ({ children }) => (
		<td className="hairline-b border-wd-hairline px-3 py-1.5 align-top">{compactEmoji(children)}</td>
	),

	blockquote: ({ children }) => (
		<blockquote className="my-3 border-l-2 border-wd-hairline pl-3 text-wd-secondary">
			{children}
		</blockquote>
	),
	hr: () => <hr className="my-4 border-wd-hairline" />,
	strong: ({ children }) => <strong className="font-medium text-wd-primary">{children}</strong>,
	del: ({ children }) => <del className="text-wd-tertiary">{children}</del>,

	// Match terminal links: a plain HTTP(S) click uses the session's Kennel Browser;
	// Option/Alt-click and non-web schemes use the system browser.
	a: MarkdownLink,

	img: ({ src, alt }) => (
		<img src={typeof src === "string" ? src : undefined} alt={alt ?? ""} className="my-2 max-w-full rounded-md hairline border-wd-hairline" />
	),
};
