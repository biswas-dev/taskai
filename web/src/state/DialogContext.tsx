import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'

export interface ConfirmOptions {
  title?: string
  message: ReactNode
  confirmLabel?: string
  cancelLabel?: string
  /** Styles the confirm button as destructive. */
  danger?: boolean
}

export type ToastKind = 'success' | 'error' | 'info'

interface Toast {
  id: number
  kind: ToastKind
  message: string
}

interface DialogApi {
  /** Resolves true when the user confirms, false when they cancel. */
  confirm: (options: ConfirmOptions | string) => Promise<boolean>
  /** Shows a short, non-blocking message in the corner of the screen. */
  notify: (message: string, kind?: ToastKind) => void
}

// Outside the provider (isolated component tests) fall back to the browser's
// own dialogs so components keep working; the app always mounts the provider.
const fallback: DialogApi = {
  confirm: async (options) => window.confirm(typeof options === 'string' ? options : String(options.message)),
  notify: (message) => window.alert(message),
}

const DialogContext = createContext<DialogApi>(fallback)

// eslint-disable-next-line react-refresh/only-export-components
export function useDialog(): DialogApi {
  return useContext(DialogContext)
}

interface PendingConfirm extends ConfirmOptions {
  resolve: (ok: boolean) => void
}

const TOAST_MS = 5000

export function DialogProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<PendingConfirm | null>(null)
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextToastId = useRef(1)

  const confirm = useCallback((options: ConfirmOptions | string) => {
    const opts = typeof options === 'string' ? { message: options } : options
    return new Promise<boolean>((resolve) => {
      setPending((current) => {
        // Only one confirmation at a time; a newer request cancels the older one.
        current?.resolve(false)
        return { ...opts, resolve }
      })
    })
  }, [])

  const notify = useCallback((message: string, kind: ToastKind = 'info') => {
    const id = nextToastId.current++
    setToasts((list) => [...list, { id, kind, message }])
    setTimeout(() => setToasts((list) => list.filter((t) => t.id !== id)), TOAST_MS)
  }, [])

  const close = (ok: boolean) => {
    pending?.resolve(ok)
    setPending(null)
  }

  const api = useMemo<DialogApi>(() => ({ confirm, notify }), [confirm, notify])

  return (
    <DialogContext.Provider value={api}>
      {children}
      {pending && <ConfirmDialog options={pending} onClose={close} />}
      {toasts.length > 0 && (
        <div className="fixed bottom-4 right-4 z-[80] flex flex-col gap-2 max-w-sm" aria-live="polite">
          {toasts.map((t) => (
            <div
              key={t.id}
              role={t.kind === 'error' ? 'alert' : 'status'}
              className={`flex items-start gap-3 px-4 py-3 rounded-lg shadow-lg border text-sm bg-dark-bg-elevated ${
                t.kind === 'error'
                  ? 'border-danger-500/40 text-danger-300'
                  : t.kind === 'success'
                    ? 'border-success-500/40 text-success-300'
                    : 'border-dark-border-subtle text-dark-text-primary'
              }`}
            >
              <span className="flex-1 break-words">{t.message}</span>
              <button
                type="button"
                aria-label="Dismiss"
                onClick={() => setToasts((list) => list.filter((x) => x.id !== t.id))}
                className="text-dark-text-tertiary hover:text-dark-text-primary"
              >
                ×
              </button>
            </div>
          ))}
        </div>
      )}
    </DialogContext.Provider>
  )
}

function ConfirmDialog({ options, onClose }: { options: ConfirmOptions; onClose: (ok: boolean) => void }) {
  const confirmRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    confirmRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose(false)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div
      className="fixed inset-0 z-[70] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
      onClick={() => onClose(false)}
    >
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="confirm-dialog-title"
        aria-describedby="confirm-dialog-message"
        className="w-full max-w-md bg-dark-bg-secondary rounded-xl border border-dark-border-subtle shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-6 pt-6 pb-4">
          <h3 id="confirm-dialog-title" className="text-base font-semibold text-dark-text-primary mb-2">
            {options.title ?? 'Are you sure?'}
          </h3>
          <div id="confirm-dialog-message" className="text-sm text-dark-text-secondary">
            {options.message}
          </div>
        </div>
        <div className="flex justify-end gap-2 px-6 py-4 border-t border-dark-border-subtle bg-dark-bg-primary/50">
          <button
            type="button"
            onClick={() => onClose(false)}
            className="px-4 py-2 text-sm font-medium text-dark-text-secondary bg-dark-bg-tertiary hover:bg-dark-bg-tertiary/80 rounded-lg transition-colors"
          >
            {options.cancelLabel ?? 'Cancel'}
          </button>
          <button
            ref={confirmRef}
            type="button"
            onClick={() => onClose(true)}
            className={`px-4 py-2 text-sm font-medium text-white rounded-lg transition-colors ${
              options.danger ? 'bg-danger-500 hover:bg-danger-600' : 'bg-primary-500 hover:bg-primary-600'
            }`}
          >
            {options.confirmLabel ?? 'Confirm'}
          </button>
        </div>
      </div>
    </div>
  )
}
