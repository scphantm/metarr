import { useEffect, useState, type ChangeEvent } from "react";
import { Form, Input, InputNumber, Select, Space } from "antd";

import { SaveIndicator } from "./SaveState";
import { useSaveState } from "./useSaveState";

/*
 * Edit-in-place fields, each a genuine antd form control at all times rather
 * than a text/input toggle — commits on blur or Enter, Escape reverts the
 * draft. Nothing here knows how to save: each field is handed an onSave that
 * performs the write, and useSaveState owns what happens between accepting
 * it and the server confirming it.
 */

type CommonProps = {
  label: string;
  queryKey: readonly unknown[];
  disabled?: boolean;
};

export function EditableText({
  value,
  onSave,
  label,
  queryKey,
  placeholder = "Not set",
  monospace = false,
  secret = false,
  multiline = false,
  validate,
  disabled,
}: CommonProps & {
  value: string;
  onSave: (next: string) => Promise<unknown>;
  placeholder?: string;
  monospace?: boolean;
  // secret masks the value behind antd Input.Password's own reveal toggle.
  // The config API returns API keys in cleartext, so anything
  // credential-shaped is masked by default rather than sitting on screen for
  // whoever walks past.
  secret?: boolean;
  multiline?: boolean;
  validate?: (next: string) => string | null;
}) {
  const { state, error, displayValue, save, dismissError } =
    useSaveState<string>({ serverValue: value, queryKey });

  const [draft, setDraft] = useState(displayValue);
  const [focused, setFocused] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);

  // Kept in sync when the confirmed server value changes underneath an
  // untouched field (e.g. another tab wrote it) — but never while the user
  // has the field open with unsaved keystrokes. Reconciling external state
  // into the draft is exactly what this effect is for.
  useEffect(() => {
    // eslint-disable-next-line @eslint-react/set-state-in-effect
    if (!focused) setDraft(displayValue);
  }, [displayValue, focused]);

  async function commit() {
    const next = draft.trim();
    if (next === displayValue) return;

    const problem = validate?.(next) ?? null;
    if (problem) {
      setValidationError(problem);
      return;
    }
    setValidationError(null);
    await save(next, () => onSave(next));
  }

  function revert() {
    setDraft(displayValue);
    setValidationError(null);
  }

  const onChange = (
    event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
  ) => setDraft(event.target.value);

  const monospaceClass = monospace ? "editable-field-mono" : "";

  return (
    <Form.Item
      validateStatus={validationError ? "error" : undefined}
      help={validationError ?? undefined}
      style={{ marginBottom: 0 }}
    >
      <Space direction="vertical" size={2} style={{ width: "100%" }}>
        {multiline ? (
          <Input.TextArea
            aria-label={label}
            className={monospaceClass}
            value={draft}
            placeholder={placeholder}
            disabled={disabled}
            rows={3}
            onChange={onChange}
            onFocus={() => setFocused(true)}
            onBlur={() => {
              setFocused(false);
              void commit();
            }}
            onKeyDown={(event) => {
              if (event.key === "Escape") revert();
            }}
          />
        ) : secret ? (
          <Input.Password
            aria-label={label}
            className={monospaceClass}
            value={draft}
            placeholder={placeholder}
            disabled={disabled}
            onChange={onChange}
            onFocus={() => setFocused(true)}
            onBlur={() => {
              setFocused(false);
              void commit();
            }}
            onPressEnter={() => void commit()}
            onKeyDown={(event) => {
              if (event.key === "Escape") revert();
            }}
          />
        ) : (
          <Input
            aria-label={label}
            className={monospaceClass}
            value={draft}
            placeholder={placeholder}
            disabled={disabled}
            onChange={onChange}
            onFocus={() => setFocused(true)}
            onBlur={() => {
              setFocused(false);
              void commit();
            }}
            onPressEnter={() => void commit()}
            onKeyDown={(event) => {
              if (event.key === "Escape") revert();
            }}
          />
        )}
        <SaveIndicator
          state={state}
          error={error}
          onDismissError={dismissError}
        />
      </Space>
    </Form.Item>
  );
}

export function EditableNumber({
  value,
  onSave,
  label,
  queryKey,
  min,
  disabled,
  validate,
}: CommonProps & {
  value: number;
  onSave: (next: number) => Promise<unknown>;
  min?: number;
  validate?: (next: number) => string | null;
}) {
  const { state, error, displayValue, save, dismissError } =
    useSaveState<number>({ serverValue: value, queryKey });

  const [draft, setDraft] = useState<number>(displayValue);
  const [focused, setFocused] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);

  // Same external-state reconciliation as EditableText above: adopt a new
  // server value only while the field is untouched.
  useEffect(() => {
    // eslint-disable-next-line @eslint-react/set-state-in-effect
    if (!focused) setDraft(displayValue);
  }, [displayValue, focused]);

  async function commit() {
    if (!Number.isFinite(draft) || !Number.isInteger(draft)) {
      setValidationError("Must be a whole number");
      return;
    }
    if (min !== undefined && draft < min) {
      setValidationError(`Must be ${min} or more`);
      return;
    }
    const problem = validate?.(draft) ?? null;
    if (problem) {
      setValidationError(problem);
      return;
    }
    setValidationError(null);
    if (draft === displayValue) return;
    await save(draft, () => onSave(draft));
  }

  return (
    <Form.Item
      validateStatus={validationError ? "error" : undefined}
      help={validationError ?? undefined}
      style={{ marginBottom: 0 }}
    >
      <Space direction="vertical" size={2}>
        <InputNumber
          aria-label={label}
          value={draft}
          min={min}
          disabled={disabled}
          onChange={(next) => setDraft(next ?? 0)}
          onFocus={() => setFocused(true)}
          onBlur={() => {
            setFocused(false);
            void commit();
          }}
          onPressEnter={() => void commit()}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              setDraft(displayValue);
              setValidationError(null);
            }
          }}
        />
        <SaveIndicator
          state={state}
          error={error}
          onDismissError={dismissError}
        />
      </Space>
    </Form.Item>
  );
}

export function EditableSelect({
  value,
  options,
  onSave,
  label,
  queryKey,
  disabled,
}: CommonProps & {
  value: string;
  options: readonly string[];
  onSave: (next: string) => Promise<unknown>;
}) {
  const { state, error, displayValue, save, dismissError } =
    useSaveState<string>({ serverValue: value, queryKey });

  // A stored value outside the vocabulary still has to be selectable, or the
  // select would silently rewrite it on the next save.
  const selectOptions = options.includes(displayValue)
    ? options
    : [displayValue, ...options];

  return (
    <Form.Item style={{ marginBottom: 0 }}>
      <Space direction="vertical" size={2}>
        <Select
          aria-label={label}
          value={displayValue}
          disabled={disabled}
          style={{ minWidth: 160 }}
          options={selectOptions.map((option) => ({
            value: option,
            label: option || "—",
          }))}
          onChange={(next) => {
            if (next !== displayValue) {
              void save(next, () => onSave(next));
            }
          }}
        />
        <SaveIndicator
          state={state}
          error={error}
          onDismissError={dismissError}
        />
      </Space>
    </Form.Item>
  );
}
