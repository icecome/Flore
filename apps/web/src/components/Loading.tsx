import { Loader2 } from './icons';

interface Props {
  text?: string;
  fullHeight?: boolean;
  size?: number;
}

export default function Loading({ text, fullHeight = false, size = 28 }: Props) {
  return (
    <div
      className={`flex flex-col items-center justify-center gap-3 text-secondary ${
        fullHeight ? 'h-full min-h-[160px] p-0' : 'p-8'
      }`}
      role="status"
      aria-live="polite"
      aria-busy="true"
    >
      <span className="inline-flex text-primary" style={{ animation: 'spin-fixed 1.5s linear infinite' }}>
        <Loader2 size={size} />
      </span>
      {text && <span className="text-sm text-secondary leading-relaxed">{text}</span>}
    </div>
  );
}
