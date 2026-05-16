import { Button } from "@/components/ui/button";

export function GovernanceActionButton({
  buttonLabel,
  disabled,
  onClick,
}: {
  buttonLabel: string;
  disabled: boolean;
  onClick: () => void;
}) {
  return (
    <div className="inline-action-cell">
      <Button
        variant="outline"
        size="sm"
        type="button"
        onClick={onClick}
        disabled={disabled}
      >
        {buttonLabel}
      </Button>
    </div>
  );
}
