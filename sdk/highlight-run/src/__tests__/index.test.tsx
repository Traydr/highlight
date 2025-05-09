import { H } from 'highlight.run'
import { HighlightPublicInterface } from '../client'
import { LDClientMin } from '../integrations/launchdarkly/types/LDClient'

// Don't run tests for now. Need to move code from firstload to client for backend errors.
describe.skip('should work outside of the browser in unit test', () => {
	let highlight: HighlightPublicInterface

	beforeEach(() => {
		jest.useFakeTimers()
		highlight = H
	})

	afterEach(() => {
		jest.useRealTimers()
	})

	it('should handle init', () => {
		highlight.init('test')
	})

	it('should handle consumeError', () => {
		const error = new Error('test error')
		highlight.consumeError(error)
	})

	it('should handle error', () => {
		highlight.error('test message')
	})

	it('should handle track', () => {
		highlight.track('test message', {})
	})

	it('should handle start', () => {
		highlight.init('test', { manualStart: true })

		highlight.start()
	})

	it('should handle stop', () => {
		highlight.stop()
	})

	it('should handle identify', () => {
		highlight.identify('123', {})
	})

	it('should handle getSessionURL', () => {
		highlight.getSessionURL()
	})

	it('should handle getSessionDetails', () => {
		highlight.getSessionDetails()
	})
})

describe('LD integration', () => {
	let highlight: HighlightPublicInterface

	beforeEach(() => {
		jest.useFakeTimers()
		highlight = H
	})

	afterEach(() => {
		jest.useRealTimers()
	})

	it('should handle register', () => {
		const client: LDClientMin = {
			track: jest.fn(),
			identify: jest.fn(),
			addHook: jest.fn(),
		}
		highlight.registerLD(client)

		expect(client.addHook).toHaveBeenCalled()
		expect(client.identify).not.toHaveBeenCalled()
		expect(client.track).not.toHaveBeenCalled()
	})
})
