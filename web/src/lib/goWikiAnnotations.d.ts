// Type declarations for window.GoWikiAnnotations (loaded from /api/wiki/annotations.js)

interface GoWikiAnnotationData {
  startOffset: number
  endOffset: number
  selectedText: string
  color: string
}

interface GoWikiAnnotationCallbacks {
  onAnnotationCreate?: (data: GoWikiAnnotationData) => void
  onAnnotationClick?: (annotationId: number) => void
}

interface GoWikiAnnotation {
  id: number
  start_offset: number
  end_offset: number
  color: string
  resolved?: boolean
}

interface GoWikiAnnotationsAPI {
  /** Attach mouseup listener + color picker to previewEl. Returns a detach function. */
  attach(previewEl: HTMLElement, callbacks: GoWikiAnnotationCallbacks): () => void
  /** Inject <mark data-ann-id> elements into previewEl for each annotation. */
  apply(previewEl: HTMLElement, annotations: GoWikiAnnotation[], selectedId?: number | null): void
}

declare global {
  interface Window {
    GoWikiAnnotations?: GoWikiAnnotationsAPI
  }
}

export {}
