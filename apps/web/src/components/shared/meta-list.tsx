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
}: {
  className?: string;
  rows: readonly MetaListRow[];
}) {
  return (
    <dl className={["meta-list", className].filter(Boolean).join(" ")}>
      {rows.map(([label, value, valueClassName, rowClassName], index) => (
        <div className={rowClassName} key={`${String(label)}-${index}`}>
          <dt>{label}</dt>
          <dd className={valueClassName}>{value}</dd>
        </div>
      ))}
    </dl>
  );
}
