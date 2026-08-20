export function Metrics({ items }: { items: Array<{ label: string; value: number | string; detail?: string }> }) {
  return <dl className="metric-strip">{items.map((item) => <div key={item.label}><dt>{item.label}</dt><dd>{item.value}</dd>{item.detail ? <span>{item.detail}</span> : null}</div>)}</dl>;
}
