//go:build pinetime

package main

import (
	"device/arm"
	"device/nrf"
	"machine"
	"runtime/interrupt"
	"runtime/volatile"
	"time"
)

var buttonWakePending volatile.Register8
var idleTimerExpired volatile.Register8

// RTC0 belongs to NimBLE and RTC1 to TinyGo. RTC2 is a one-shot wake timer.
// This application has one Go task: direct WFE is used only while the UI is
// asleep and the radio has fully stopped. Ordinary time.Sleep cannot return
// early for an input IRQ in TinyGo 0.42's nRF runtime.
func configureIdleInterrupts() {
	nrf.GPIOTE.INTENCLR.Set(nrf.GPIOTE_INTENCLR_PORT)
	nrf.GPIOTE.EVENTS_PORT.Set(0)
	nrf.RTC2.TASKS_STOP.Set(1)
	nrf.RTC2.PRESCALER.Set(0)
	nrf.RTC2.INTENCLR.Set(0xffffffff)
	nrf.RTC2.EVENTS_COMPARE[0].Set(0)
	timerIRQ := interrupt.New(nrf.IRQ_RTC2, func(interrupt.Interrupt) {
		nrf.RTC2.INTENCLR.Set(nrf.RTC_INTENCLR_COMPARE0)
		nrf.RTC2.EVENTS_COMPARE[0].Set(0)
		idleTimerExpired.Set(1)
		arm.Asm("sev")
	})
	timerIRQ.SetPriority(0xc0)
	timerIRQ.Enable()
}

func (d *pineTimeDisplay) waitAsleep(duration time.Duration) {
	duration = min(duration, idleWatchdogInterval(nrf.WDT.RUNSTATUS.Get() != 0, nrf.WDT.CRV.Get()))
	ticks := uint32(max(3, (duration.Nanoseconds()*32768+999999999)/1000000000))
	state := interrupt.Disable()
	// TinyGo owns the GPIOTE handler and handles IN channels only. Temporarily
	// mask that handler and use ARM SEVONPEND: a pending GPIO interrupt wakes
	// WFE without executing an ISR. Touch events remain latched for its handler.
	gpioIRQEnabled := arm.NVIC.ISER[0].Get()&(1<<nrf.IRQ_GPIOTE) != 0
	arm.DisableIRQ(nrf.IRQ_GPIOTE)
	arm.ClearPendingIRQ(nrf.IRQ_GPIOTE)
	oldSCR := arm.SCB.SCR.Get()
	arm.SCB.SCR.Set(oldSCR | arm.SCB_SCR_SEVONPEND)
	buttonWakePending.Set(0)
	idleTimerExpired.Set(0)
	// Unlike pulsed polling, GPIO sense needs the button circuit energized
	// throughout sleep (~34 uA). Avoids 50 CPU wakeups/sec; measure net saving.
	for i := 0; i < 8; i++ {
		machine.BUTTON_OUT.High()
	}
	nrf.GPIOTE.INTENCLR.Set(nrf.GPIOTE_INTENCLR_PORT)
	nrf.P0.PIN_CNF[machine.BUTTON_IN].SetBits(nrf.GPIO_PIN_CNF_SENSE_High << nrf.GPIO_PIN_CNF_SENSE_Pos)
	nrf.GPIOTE.EVENTS_PORT.Set(0)
	nrf.GPIOTE.INTENSET.Set(nrf.GPIOTE_INTENSET_PORT)
	if machine.BUTTON_IN.Get() {
		buttonWakePending.Set(1)
	}
	nrf.RTC2.EVENTS_COMPARE[0].Set(0)
	nrf.RTC2.CC[0].Set((nrf.RTC2.COUNTER.Get() + ticks) & 0xffffff)
	nrf.RTC2.INTENSET.Set(nrf.RTC_INTENSET_COMPARE0)
	nrf.RTC2.TASKS_START.Set(1)
	interrupt.Restore(state)
	for buttonWakePending.Get() == 0 && nrf.GPIOTE.EVENTS_PORT.Get() == 0 && idleTimerExpired.Get() == 0 && d.touch.pending.Get() == 0 && !touchHardwarePending() {
		arm.Asm("dsb")
		arm.Asm("wfe")
	}
	state = interrupt.Disable()
	// Preserve a short press arriving during timer wake/cleanup, even if its
	// interrupt has not yet run. The next readButton consumes the latch.
	if nrf.GPIOTE.EVENTS_PORT.Get() != 0 || machine.BUTTON_IN.Get() {
		buttonWakePending.Set(1)
	}
	nrf.P0.PIN_CNF[machine.BUTTON_IN].ClearBits(nrf.GPIO_PIN_CNF_SENSE_Msk)
	nrf.GPIOTE.INTENCLR.Set(nrf.GPIOTE_INTENCLR_PORT)
	nrf.GPIOTE.EVENTS_PORT.Set(0)
	machine.BUTTON_OUT.Low()
	nrf.RTC2.INTENCLR.Set(nrf.RTC_INTENCLR_COMPARE0)
	nrf.RTC2.TASKS_STOP.Set(1)
	nrf.RTC2.EVENTS_COMPARE[0].Set(0)
	arm.SCB.SCR.Set(oldSCR)
	arm.ClearPendingIRQ(nrf.IRQ_GPIOTE)
	if gpioIRQEnabled {
		arm.EnableIRQ(nrf.IRQ_GPIOTE)
	}
	interrupt.Restore(state)
}

func touchHardwarePending() bool {
	enabled := nrf.GPIOTE.INTENSET.Get() & 0xff
	for i := range nrf.GPIOTE.EVENTS_IN {
		if enabled&(1<<uint(i)) != 0 && nrf.GPIOTE.EVENTS_IN[i].Get() != 0 {
			return true
		}
	}
	return false
}
