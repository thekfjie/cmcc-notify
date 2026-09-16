import * as React from "react"
import { createPortal } from "react-dom"
import { Check, ChevronDown } from "lucide-react"

import { cn } from "@/lib/utils"

export type SelectOption = {
  value: string
  label: string
  description?: string
  disabled?: boolean
}

type SelectProps = {
  id: string
  value: string
  options: SelectOption[]
  onValueChange: (value: string) => void
  placeholder?: string
  emptyLabel?: string
  disabled?: boolean
  icon?: React.ReactNode
  endAdornment?: React.ReactNode
  className?: string
}

function Select({
  id,
  value,
  options,
  onValueChange,
  placeholder = "请选择",
  emptyLabel = "暂无可选项",
  disabled = false,
  icon,
  endAdornment,
  className,
}: SelectProps) {
  const rootRef = React.useRef<HTMLDivElement>(null)
  const menuRef = React.useRef<HTMLDivElement>(null)
  const [open, setOpen] = React.useState(false)
  const [menuStyle, setMenuStyle] = React.useState<React.CSSProperties>({})
  const selectedIndex = options.findIndex((option) => option.value === value)
  const [activeIndex, setActiveIndex] = React.useState(selectedIndex >= 0 ? selectedIndex : 0)
  const listboxID = `${id}-listbox`
  const selected = selectedIndex >= 0 ? options[selectedIndex] : undefined
  const unavailable = disabled || options.length === 0

  React.useEffect(() => {
    if (!open) return
    const closeOnOutsidePointer = (event: PointerEvent) => {
      const target = event.target as Node
      if (!rootRef.current?.contains(target) && !menuRef.current?.contains(target)) setOpen(false)
    }
    document.addEventListener("pointerdown", closeOnOutsidePointer)
    return () => document.removeEventListener("pointerdown", closeOnOutsidePointer)
  }, [open])

  React.useLayoutEffect(() => {
    if (!open || !rootRef.current) return
    const positionMenu = () => {
      const rect = rootRef.current?.getBoundingClientRect()
      if (!rect) return
      const edge = 12
      const gap = 8
      const below = window.innerHeight - rect.bottom - edge - gap
      const above = rect.top - edge - gap
      const opensUp = below < 180 && above > below
      const maxHeight = Math.max(96, Math.min(300, opensUp ? above : below))
      const width = Math.min(rect.width, window.innerWidth - edge * 2)
      const left = Math.max(edge, Math.min(rect.left, window.innerWidth - width - edge))
      setMenuStyle({
        left,
        width,
        maxHeight,
        top: opensUp ? "auto" : rect.bottom + gap,
        bottom: opensUp ? window.innerHeight - rect.top + gap : "auto",
        transformOrigin: opensUp ? "bottom" : "top",
      })
    }
    positionMenu()
    window.addEventListener("resize", positionMenu)
    window.addEventListener("scroll", positionMenu, true)
    return () => {
      window.removeEventListener("resize", positionMenu)
      window.removeEventListener("scroll", positionMenu, true)
    }
  }, [open])

  React.useEffect(() => {
    if (!open) return
    setActiveIndex(selectedIndex >= 0 ? selectedIndex : options.findIndex((option) => !option.disabled))
  }, [open, options, selectedIndex])

  const moveActive = (direction: 1 | -1) => {
    if (!options.length) return
    let next = activeIndex
    for (let attempts = 0; attempts < options.length; attempts += 1) {
      next = (next + direction + options.length) % options.length
      if (!options[next]?.disabled) {
        setActiveIndex(next)
        return
      }
    }
  }

  const selectOption = (option: SelectOption) => {
    if (option.disabled) return
    onValueChange(option.value)
    setOpen(false)
  }

  const onKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (unavailable) return
    if (event.key === "Escape") {
      setOpen(false)
      return
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      if (!open) setOpen(true)
      else moveActive(event.key === "ArrowDown" ? 1 : -1)
      return
    }
    if ((event.key === "Enter" || event.key === " ") && event.target === rootRef.current?.querySelector(".select-trigger")) {
      event.preventDefault()
      if (open && options[activeIndex]) selectOption(options[activeIndex])
      else setOpen(true)
    }
  }

  return (
    <div ref={rootRef} className={cn("select-control", open && "open", className)} onKeyDown={onKeyDown}>
      <button
        id={id}
        type="button"
        className="select-trigger"
        aria-haspopup="listbox"
        aria-controls={listboxID}
        aria-expanded={open}
        aria-activedescendant={open && activeIndex >= 0 ? `${id}-option-${activeIndex}` : undefined}
        disabled={unavailable}
        onClick={() => setOpen((current) => !current)}
      >
        {icon && <span className="select-leading">{icon}</span>}
        <span className={cn("select-value", !selected && "placeholder")}>
          <strong>{selected?.label ?? (options.length ? placeholder : emptyLabel)}</strong>
          {selected?.description && <small>{selected.description}</small>}
        </span>
        {endAdornment && <span className="select-adornment">{endAdornment}</span>}
        <ChevronDown className="select-chevron" />
      </button>
      {open && createPortal(
        <div ref={menuRef} id={listboxID} className="select-menu" style={menuStyle} role="listbox" aria-labelledby={id}>
          {options.map((option, index) => (
            <button
              id={`${id}-option-${index}`}
              type="button"
              role="option"
              aria-selected={option.value === value}
              className={cn("select-option", index === activeIndex && "active", option.value === value && "selected")}
              disabled={option.disabled}
              key={option.value}
              onPointerMove={() => setActiveIndex(index)}
              onClick={() => selectOption(option)}
            >
              <span>
                <strong>{option.label}</strong>
                {option.description && <small>{option.description}</small>}
              </span>
              {option.value === value && <Check />}
            </button>
          ))}
        </div>,
        document.body,
      )}
    </div>
  )
}

export { Select }
