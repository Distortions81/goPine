// Cooperative NimBLE port. Host work runs on the UI goroutine; radio timing
// remains in NimBLE's interrupt handlers. No Go callbacks run in interrupts.
#include <assert.h>
#include <string.h>
#include <nrf.h>
#include "nimble/nimble_npl.h"

static struct ble_npl_eventq *queues;
static struct ble_npl_callout *callouts;
static uint32_t last_ticks;
static uint64_t total_ticks;
static uint32_t vectors[64] __attribute__((aligned(256)));
static bool vectors_ready;

// A failed invariant cannot safely resume a radio stack. Keep MCUboot intact;
// resetting an unconfirmed trial allows its normal rollback mechanism.
void __attribute__((noreturn)) __assert_func(const char *file, int line, const char *fn, const char *expr) {
    (void)file; (void)line; (void)fn; (void)expr;
    NVIC_SystemReset();
    for (;;) {}
}

uint32_t ble_npl_hw_enter_critical(void) { uint32_t s=__get_PRIMASK();__disable_irq();return s; }
void ble_npl_hw_exit_critical(uint32_t s) { __set_PRIMASK(s); }
bool ble_npl_hw_is_in_critical(void) {return __get_PRIMASK()!=0;}
void ble_npl_hw_set_isr(int irq,void (*fn)(void)) {
    assert(irq>=0 && irq<48);
    uint32_t s=ble_npl_hw_enter_critical();
    if (!vectors_ready) {memcpy(vectors,(void*)SCB->VTOR,sizeof(vectors));SCB->VTOR=(uint32_t)vectors;__DSB();__ISB();vectors_ready=true;}
    vectors[irq+16]=(uint32_t)fn;
    __DSB();__ISB();ble_npl_hw_exit_critical(s);
}
bool ble_npl_os_started(void){return true;}
void *ble_npl_get_current_task_id(void){return (void*)1;}
uint32_t ble_npl_time_get(void){
    uint32_t s=ble_npl_hw_enter_critical();
    uint32_t now=NRF_RTC0->COUNTER;
    total_ticks+=(now-last_ticks)&0xffffff;last_ticks=now;
    uint32_t ms=(uint32_t)(total_ticks*1000/32768);
    ble_npl_hw_exit_critical(s);return ms;
}
ble_npl_error_t ble_npl_time_ms_to_ticks(uint32_t ms,ble_npl_time_t *out){*out=ms;return 0;}
ble_npl_error_t ble_npl_time_ticks_to_ms(ble_npl_time_t ticks,uint32_t *out){*out=ticks;return 0;}
uint32_t ble_npl_time_ms_to_ticks32(uint32_t v){return v;}
uint32_t ble_npl_time_ticks_to_ms32(uint32_t v){return v;}
void ble_npl_eventq_init(struct ble_npl_eventq *q){
    for(struct ble_npl_eventq *p=queues;p;p=p->next){if(p==q)return;}
    memset(q,0,sizeof(*q));q->next=queues;queues=q;
}
void ble_npl_event_init(struct ble_npl_event *ev,ble_npl_event_fn *fn,void *arg){memset(ev,0,sizeof(*ev));ev->fn=fn;ev->arg=arg;}
bool ble_npl_event_is_queued(struct ble_npl_event *ev){return ev->queued;}
void *ble_npl_event_get_arg(struct ble_npl_event *ev){return ev->arg;}
void ble_npl_event_set_arg(struct ble_npl_event *ev,void *arg){ev->arg=arg;}
void ble_npl_event_run(struct ble_npl_event *ev){ev->fn(ev);}
bool ble_npl_eventq_is_empty(struct ble_npl_eventq *q){return q->head==NULL;}
void ble_npl_eventq_put(struct ble_npl_eventq *q,struct ble_npl_event *ev){
    uint32_t s=ble_npl_hw_enter_critical();
    if(!ev->queued){ev->queued=true;ev->next=NULL;if(q->tail)q->tail->next=ev;else q->head=ev;q->tail=ev;}
    ble_npl_hw_exit_critical(s);
}
struct ble_npl_event *ble_npl_eventq_get(struct ble_npl_eventq *q,ble_npl_time_t timeout){
    (void)timeout;uint32_t s=ble_npl_hw_enter_critical();struct ble_npl_event *ev=q->head;
    if(ev){q->head=ev->next;if(!q->head)q->tail=NULL;ev->next=NULL;ev->queued=false;}
    ble_npl_hw_exit_critical(s);return ev;
}
void ble_npl_eventq_remove(struct ble_npl_eventq *q,struct ble_npl_event *ev){
    uint32_t s=ble_npl_hw_enter_critical();struct ble_npl_event *prev=NULL;
    for(struct ble_npl_event *p=q->head;p;prev=p,p=p->next){if(p==ev){if(prev)prev->next=p->next;else q->head=p->next;if(q->tail==p)q->tail=prev;p->queued=false;p->next=NULL;break;}}
    ble_npl_hw_exit_critical(s);
}
ble_npl_error_t ble_npl_mutex_init(struct ble_npl_mutex *m){m->depth=0;return 0;}
ble_npl_error_t ble_npl_mutex_pend(struct ble_npl_mutex *m,uint32_t t){(void)t;m->depth++;return 0;}
ble_npl_error_t ble_npl_mutex_release(struct ble_npl_mutex *m){assert(m->depth);m->depth--;return 0;}
ble_npl_error_t ble_npl_sem_init(struct ble_npl_sem *s,uint16_t n){s->count=n;return 0;}
ble_npl_error_t ble_npl_sem_release(struct ble_npl_sem *s){uint32_t p=ble_npl_hw_enter_critical();s->count++;ble_npl_hw_exit_critical(p);return 0;}
uint16_t ble_npl_sem_get_count(struct ble_npl_sem *s){return s->count;}
ble_npl_error_t ble_npl_sem_pend(struct ble_npl_sem *s,uint32_t timeout){
    uint32_t start=ble_npl_time_get();
    do{uint32_t p=ble_npl_hw_enter_critical();if(s->count){s->count--;ble_npl_hw_exit_critical(p);return 0;}ble_npl_hw_exit_critical(p);gopine_ble_pump();}while((uint32_t)(ble_npl_time_get()-start)<timeout);
    return BLE_NPL_TIMEOUT;
}
void ble_npl_callout_init(struct ble_npl_callout *co,struct ble_npl_eventq *q,ble_npl_event_fn *fn,void *arg){
    bool found=false;for(struct ble_npl_callout *p=callouts;p;p=p->next){if(p==co)found=true;}
    co->active=false;co->evq=q;ble_npl_event_init(&co->ev,fn,arg);
    if(!found){co->next=callouts;callouts=co;}
}
ble_npl_error_t ble_npl_callout_reset(struct ble_npl_callout *co,uint32_t ticks){uint32_t s=ble_npl_hw_enter_critical();co->deadline=ble_npl_time_get()+ticks;co->active=true;ble_npl_hw_exit_critical(s);return 0;}
void ble_npl_callout_stop(struct ble_npl_callout *co){uint32_t s=ble_npl_hw_enter_critical();co->active=false;if(co->evq)ble_npl_eventq_remove(co->evq,&co->ev);ble_npl_hw_exit_critical(s);}
bool ble_npl_callout_is_active(struct ble_npl_callout *co){return co->active;}
uint32_t ble_npl_callout_get_ticks(struct ble_npl_callout *co){return co->deadline;}
uint32_t ble_npl_callout_remaining_ticks(struct ble_npl_callout *co,uint32_t now){int32_t n=(int32_t)(co->deadline-now);return n>0?(uint32_t)n:0;}
void ble_npl_callout_set_arg(struct ble_npl_callout *co,void *arg){co->ev.arg=arg;}
void gopine_ble_pump(void){
    uint32_t now=ble_npl_time_get();
    for(struct ble_npl_callout *co=callouts;co;co=co->next){
        bool run=false;
        uint32_t s=ble_npl_hw_enter_critical();
        if(co->active && (int32_t)(now-co->deadline)>=0){co->active=false;if(co->evq)ble_npl_eventq_put(co->evq,&co->ev);else run=true;}
        ble_npl_hw_exit_critical(s);
        if(run)ble_npl_event_run(&co->ev);
    }
    for(struct ble_npl_eventq *q=queues;q;q=q->next){if(q->busy)continue;q->busy=true;for(int i=0;i<16;i++){struct ble_npl_event *ev=ble_npl_eventq_get(q,0);if(!ev)break;ble_npl_event_run(ev);}q->busy=false;}
}
void ble_npl_time_delay(uint32_t ticks){uint32_t start=ble_npl_time_get();while((uint32_t)(ble_npl_time_get()-start)<ticks)gopine_ble_pump();}
