import type { ReactNode } from "react";

export type MetaListRow = readonly [
  label: ReactNode,
  value: ReactNode,
  valueClassName?: string,
  rowClassName?: string,
];

export function MetaList({
  className = "vertical",
  rows,
}: Readonly<{
  className?: string;
  rows: readonly MetaListRow[];
}>) {
  return (
    <dl className={["meta-list", className].filter(Boolean).join(" ")}>
      {rows.map(([label, value, valueClassName, rowClassName]) => (
        <div
          className={rowClassName}
          key={rowKey(label, value, rowClassName, valueClassName)}
        >
          <dt>{label}</dt>
          <dd className={valueClassName}>{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function rowKey(
  label: MetaListRow[0],
  value: MetaListRow[1],
  rowClassName?: string,
  valueClassName?: string,
) {
  const labelText = primitiveNodeKeyPart(label);
  const valueText = primitiveNodeKeyPart(value);
  const classText = rowClassName || valueClassName || "";
  return `${classText}::${labelText}::${valueText}`;
}

function primitiveNodeKeyPart(node: ReactNode) {
  if (
    typeof node === "string" ||
    typeof node === "number" ||
    typeof node === "boolean"
  ) {
    return String(node);
  }
  if (node === null || node === undefined) {
    return "";
  }
  return "node";
}
