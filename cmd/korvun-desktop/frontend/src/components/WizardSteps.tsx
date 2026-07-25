// The step indicator shared by the channel wizard and the onboarding (review
// finding: the markup was copy-pasted between the two). `step` is 1-based;
// labels are prefixed with their number ("1 · Canal").
export function WizardSteps({
  step,
  labels,
}: {
  step: number
  labels: readonly string[]
}): React.JSX.Element {
  return (
    <div className="wiz-steps">
      {labels.map((label, i) => (
        <span key={label} style={{ display: 'contents' }}>
          {i > 0 && <span className="wiz-sep" />}
          <span className={`wiz-step ${step === i + 1 ? 'wiz-step-on' : ''}`}>
            {i + 1} · {label}
          </span>
        </span>
      ))}
    </div>
  )
}
