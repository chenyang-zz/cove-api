import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type RefObject } from 'react'

export type AppRoute = 'chat' | 'profile'

type NavigationDirection = 'idle' | 'forward' | 'back'

type NavigationHistoryState = {
  coveNavigation: true
  route: AppRoute
  session: string
}

type NavigationState = {
  route: AppRoute
  direction: NavigationDirection
}

type EdgeBackGestureOptions = {
  enabled: boolean
  onBack: () => void
  stageRef: RefObject<HTMLElement | null>
}

type DragState = {
  pointerId: number
  startX: number
  startY: number
  startTime: number
  width: number
  lastX: number
  lastTime: number
  progress: number
}

const rootRoute: AppRoute = 'chat'
const edgeActivationWidth = 24
const completionThreshold = 0.35
const completionVelocity = 0.5

function createSessionKey() {
  return `navigation-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function historyState(session: string, route: AppRoute): NavigationHistoryState {
  return { coveNavigation: true, route, session }
}

function isNavigationHistoryState(value: unknown, session: string): value is NavigationHistoryState {
  if (!value || typeof value !== 'object') {
    return false
  }
  const candidate = value as Partial<NavigationHistoryState>
  return candidate.coveNavigation === true
    && candidate.session === session
    && (candidate.route === 'chat' || candidate.route === 'profile')
}

export function useAppNavigation() {
  const sessionRef = useRef(createSessionKey())
  const routeRef = useRef<AppRoute>(rootRoute)
  const [navigation, setNavigation] = useState<NavigationState>({ route: rootRoute, direction: 'idle' })

  const applyRoute = useCallback((route: AppRoute, direction: NavigationDirection) => {
    routeRef.current = route
    setNavigation({ route, direction })
  }, [])

  useEffect(() => {
    const session = sessionRef.current
    const currentState = window.history.state
    if (isNavigationHistoryState(currentState, session)) {
      applyRoute(currentState.route, 'idle')
    } else {
      window.history.replaceState(historyState(session, rootRoute), '')
    }

    function syncHistoryRoute(event: PopStateEvent) {
      const nextRoute = isNavigationHistoryState(event.state, session) ? event.state.route : rootRoute
      applyRoute(nextRoute, nextRoute === 'profile' ? 'forward' : 'back')
    }

    window.addEventListener('popstate', syncHistoryRoute)
    return () => window.removeEventListener('popstate', syncHistoryRoute)
  }, [applyRoute])

  const navigate = useCallback((route: AppRoute) => {
    if (routeRef.current === route) {
      return
    }
    const session = sessionRef.current
    window.history.pushState(historyState(session, route), '')
    applyRoute(route, 'forward')
  }, [applyRoute])

  const back = useCallback(() => {
    if (routeRef.current !== 'profile') {
      return
    }
    const session = sessionRef.current
    applyRoute(rootRoute, 'back')
    if (isNavigationHistoryState(window.history.state, session) && window.history.state.route === 'profile') {
      window.history.back()
      return
    }
    window.history.replaceState(historyState(session, rootRoute), '')
  }, [applyRoute])

  const reset = useCallback(() => {
    applyRoute(rootRoute, 'idle')
    window.history.replaceState(historyState(sessionRef.current, rootRoute), '')
  }, [applyRoute])

  return { ...navigation, navigate, back, reset }
}

function clearSwipeStyles(stage: HTMLElement) {
  delete stage.dataset.swipeState
  stage.style.removeProperty('--navigation-profile-offset')
  stage.style.removeProperty('--navigation-chat-offset')
  stage.style.removeProperty('--navigation-scrim-opacity')
}

function applySwipeProgress(stage: HTMLElement, progress: number) {
  stage.dataset.swipeState = 'dragging'
  stage.style.setProperty('--navigation-profile-offset', `${progress * 100}%`)
  stage.style.setProperty('--navigation-chat-offset', `${-12 + progress * 12}%`)
  stage.style.setProperty('--navigation-scrim-opacity', `${0.18 * (1 - progress)}`)
}

export function useEdgeBackGesture({ enabled, onBack, stageRef }: EdgeBackGestureOptions) {
  const dragRef = useRef<DragState | null>(null)

  const cancel = useCallback(() => {
    dragRef.current = null
    const stage = stageRef.current
    if (stage) {
      clearSwipeStyles(stage)
    }
  }, [stageRef])

  useEffect(() => cancel, [cancel])

  const onPointerDown = useCallback((event: ReactPointerEvent<HTMLElement>) => {
    if (!enabled || event.button !== 0 || event.clientX > edgeActivationWidth) {
      return
    }
    if (event.target instanceof Element && event.target.closest('button, input, textarea, select, [role="dialog"]')) {
      return
    }
    const stage = stageRef.current
    if (!stage) {
      return
    }
    const width = stage.getBoundingClientRect().width || window.innerWidth
    dragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      startTime: event.timeStamp,
      width: Math.max(1, width),
      lastX: event.clientX,
      lastTime: event.timeStamp,
      progress: 0,
    }
    event.currentTarget.setPointerCapture?.(event.pointerId)
  }, [enabled, stageRef])

  const onPointerMove = useCallback((event: ReactPointerEvent<HTMLElement>) => {
    const drag = dragRef.current
    const stage = stageRef.current
    if (!drag || !stage || drag.pointerId !== event.pointerId) {
      return
    }
    const horizontalDistance = event.clientX - drag.startX
    const verticalDistance = event.clientY - drag.startY
    if (horizontalDistance <= 0) {
      return
    }
    if (Math.abs(verticalDistance) > horizontalDistance && horizontalDistance < 18) {
      cancel()
      return
    }
    const progress = Math.min(1, horizontalDistance / drag.width)
    drag.progress = progress
    drag.lastX = event.clientX
    drag.lastTime = event.timeStamp
    applySwipeProgress(stage, progress)
    event.preventDefault()
  }, [cancel, stageRef])

  const finish = useCallback((event: ReactPointerEvent<HTMLElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) {
      return
    }
    const elapsed = Math.max(1, event.timeStamp - drag.lastTime)
    const velocity = Math.max(0, event.clientX - drag.lastX) / elapsed
    const shouldGoBack = drag.progress >= completionThreshold || velocity >= completionVelocity
    cancel()
    if (shouldGoBack) {
      onBack()
    }
  }, [cancel, onBack])

  return { onPointerDown, onPointerMove, onPointerUp: finish, onPointerCancel: cancel }
}
