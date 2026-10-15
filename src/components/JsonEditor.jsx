import React, { useMemo, useRef } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { json, jsonParseLinter } from "@codemirror/lang-json";
import { linter, lintGutter } from "@codemirror/lint";
import { EditorView, keymap } from "@codemirror/view";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags as t } from "@lezer/highlight";

// Colors come from CSS variables, so the editor follows the light/dark theme.
const theme = EditorView.theme({
  "&": { backgroundColor: "transparent", color: "rgb(var(--fg))" },
  ".cm-content": { caretColor: "rgb(var(--accent))", padding: "12px 0" },
  ".cm-cursor": { borderLeftColor: "rgb(var(--accent))", borderLeftWidth: "2px" },
  ".cm-gutters": { backgroundColor: "transparent", color: "rgb(var(--faint))", border: "none" },
  ".cm-lineNumbers .cm-gutterElement": { padding: "0 10px 0 14px", fontSize: "11px" },
  ".cm-activeLine": { backgroundColor: "rgb(var(--line) / 0.35)" },
  ".cm-activeLineGutter": { backgroundColor: "transparent", color: "rgb(var(--muted))" },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
    backgroundColor: "rgb(var(--accent) / 0.25) !important",
  },
  ".cm-matchingBracket": { backgroundColor: "rgb(var(--accent) / 0.2)", outline: "none" },
  ".cm-foldPlaceholder": { backgroundColor: "rgb(var(--line))", border: "none", color: "rgb(var(--muted))", padding: "0 6px" },
  ".cm-tooltip": { backgroundColor: "rgb(var(--elevated))", border: "1px solid rgb(var(--line))", borderRadius: "8px" },
  ".cm-diagnostic-error": { borderLeftColor: "rgb(var(--danger))" },
  ".cm-searchMatch": { backgroundColor: "rgb(var(--warn) / 0.25)" },
  ".cm-panels": { backgroundColor: "rgb(var(--elevated))", color: "rgb(var(--fg))", borderColor: "rgb(var(--line))" },
});

const highlight = HighlightStyle.define([
  { tag: t.propertyName, color: "hsl(255 85% var(--chip-l))" },
  { tag: t.string, color: "hsl(150 55% calc(var(--chip-l) - 4%))" },
  { tag: t.number, color: "hsl(28 90% calc(var(--chip-l) - 6%))" },
  { tag: [t.bool, t.null], color: "hsl(330 75% var(--chip-l))", fontWeight: "600" },
  { tag: [t.brace, t.squareBracket, t.separator, t.punctuation], color: "rgb(var(--faint))" },
]);

export function JsonEditor({ value, onChange, onSave, readOnly }) {
  // Keep extensions stable: reconfiguring CodeMirror on every render is costly.
  const saveRef = useRef(onSave);
  saveRef.current = onSave;
  const extensions = useMemo(
    () => [
      json(),
      linter(jsonParseLinter(), { delay: 250 }),
      lintGutter(),
      theme,
      syntaxHighlighting(highlight),
      EditorView.lineWrapping,
      keymap.of([
        {
          key: "Mod-s",
          preventDefault: true,
          run: () => {
            saveRef.current?.();
            return true;
          },
        },
      ]),
    ],
    []
  );

  return (
    <CodeMirror
      value={value}
      onChange={onChange}
      extensions={extensions}
      theme="none"
      readOnly={readOnly}
      height="100%"
      className="h-full"
      basicSetup={{ highlightActiveLine: true, foldGutter: true, autocompletion: false, searchKeymap: true }}
    />
  );
}
