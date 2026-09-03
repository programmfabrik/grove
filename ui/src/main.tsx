import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { SettingsPage } from './components/Settings'
import { initTheme } from './lib/theme'
import './styles.css'

initTheme()

// The settings window is the same page asked for a different view. It is a
// window of its own — opened by cmd-comma, with the application's name in its
// title bar — rather than a panel inside the dashboard, because that is where
// somebody looks for settings and no button in a web toolbar will change that.
const settings = new URLSearchParams(location.search).get('view') === 'settings'
if (settings) document.title = 'Grove Settings'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>{settings ? <SettingsPage /> : <App />}</React.StrictMode>,
)
