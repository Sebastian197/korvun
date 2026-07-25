// Shell smoke: navigation renders every section, the stopped Home carries
// the single gradient action, and the theme toggle swaps data-theme.
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { App } from './App'

describe('chrome shell', () => {
  it('renders the sidebar with all five sections and the version row', () => {
    render(<App />)
    for (const label of ['Inicio', 'Canales', 'Actividad', 'Builder', 'Ajustes']) {
      expect(screen.getByRole('button', { name: label })).toBeInTheDocument()
    }
    expect(screen.getByTestId('version').textContent).toBe('dev')
  })

  it('the stopped Home shows the hero with the one primary action', () => {
    render(<App />)
    expect(screen.getByTestId('home-parado')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Iniciar Korvun' })).toBeInTheDocument()
  })

  it('the theme toggle swaps data-theme on the document element', () => {
    render(<App />)
    expect(document.documentElement.dataset.theme).toBe('dark')
    fireEvent.click(screen.getByTestId('theme-toggle'))
    expect(document.documentElement.dataset.theme).toBe('light')
    fireEvent.click(screen.getByTestId('theme-toggle'))
    expect(document.documentElement.dataset.theme).toBe('dark')
  })

  it('navigating to a future view shows its honest empty state', () => {
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'Canales' }))
    expect(screen.getByText(/Canales estará disponible/)).toBeInTheDocument()
  })
})
