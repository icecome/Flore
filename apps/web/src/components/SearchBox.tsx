import { useState, useRef, useEffect } from 'react';
import { Search, X } from './icons';
import ContextMenu from './ContextMenu';
import { useContextMenu } from '../hooks/useContextMenu';
import { buildInputMenu } from '../utils/contextMenu';

interface Props {
  query: string;
  onSearch: (keyword: string) => void;
  onClear: () => void;
  placeholder?: string;
}

function createSubmitHandler({
  onSearch,
  onClear,
}: {
  onSearch: (keyword: string) => void;
  onClear: () => void;
}) {
  return (e: React.FormEvent) => {
    e.preventDefault();
    const keyword = (e.currentTarget as HTMLFormElement).querySelector('input')?.value?.trim() ?? '';
    if (keyword) onSearch(keyword);
    else onClear();
  };
}

export default function SearchBox({ query, onSearch, onClear, placeholder = '搜索文章...' }: Props) {
  const [input, setInput] = useState(query);
  const inputRef = useRef<HTMLInputElement>(null);
  const { menuProps, showMenu } = useContextMenu();

  useEffect(() => { setInput(query); }, [query]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (input.trim()) onSearch(input.trim());
    else onClear();
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setInput(value);
    if (value === '') onClear();
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      onClear();
      inputRef.current?.blur();
    }
  };

  return (
    <>
      <form onSubmit={handleSubmit} className="flex items-center gap-1.5 flex-1 min-w-0 px-2.5 py-1.5 rounded-md bg-input border border-border">
        <Search size={14} className="text-muted shrink-0" />
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          onContextMenu={(e) => {
            const el = inputRef.current;
            if (!el) return;
            showMenu(e, buildInputMenu(
              {
                hasSelection: el.selectionStart !== null && el.selectionEnd !== null && el.selectionStart !== el.selectionEnd,
                hasValue: input.length > 0,
                readOnly: el.readOnly,
              },
              { onClear },
            ));
          }}
          placeholder={placeholder}
          className="flex-1 min-w-0 border-none outline-none bg-transparent text-sm text-primary leading-[18px]"
        />
        {input && (
          <button type="button" onClick={onClear} className="border-none bg-transparent p-0 cursor-pointer flex items-center justify-center text-muted shrink-0" title="清空">
            <X size={14} />
          </button>
        )}
      </form>
      {menuProps && <ContextMenu {...menuProps} />}
    </>
  );
}
