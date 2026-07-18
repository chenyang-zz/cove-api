import React from 'react'
import ReactDOM from 'react-dom/client'
import './styles/global.css'
import App from './app/App'
import { NativeChatApp } from './app/NativeChatApp'
import { NativeProfileApp } from './app/NativeProfileApp'
import { NativeRegisterApp } from './app/NativeRegisterApp'
import { isNativeChatEntry, isNativeProfileEntry, isNativeRegisterEntry } from './app/nativeNavigation'

const rootApp = isNativeProfileEntry()
  ? <NativeProfileApp />
  : isNativeRegisterEntry()
    ? <NativeRegisterApp />
    : isNativeChatEntry()
      ? <NativeChatApp />
      : <App />

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    {rootApp}
  </React.StrictMode>,
)
