using System;
using System.Collections;
using System.Text;
using Fence.Bridge;
using Fence.Gym;
using Fence.Protocol;
using UnityEngine;
using UnityEngine.Networking;

namespace Fence.Mod
{
    sealed class HttpResult
    {
        public long Status;
        public string Body;
        public string Error;
        public bool TimedOut;

        public bool Transport => TimedOut || Status == 0;
    }

    // All bridge I/O. Runs as coroutines on the main thread (UnityWebRequest); never a thread, task or
    // timer, which the game's ModScanner refuses. Decisions about what to send live in BridgeSession.
    sealed class BridgeClient
    {
        const int HelloTimeoutMs = 5000;
        const int EventTimeoutMs = 3000;

        readonly BridgeSession session;
        readonly FenceLog log;
        readonly Func<string> baseUrl;
        readonly Func<HelloRequest> buildHello;
        readonly Func<IEnumerator, bool> startCoroutine;
        bool pumping;
        bool stopped;
        string lastVanillaLogged;

        public BridgeClient(BridgeSession session, FenceLog log, Func<string> baseUrl, Func<HelloRequest> buildHello, Func<IEnumerator, bool> startCoroutine)
        {
            this.session = session;
            this.log = log;
            this.baseUrl = baseUrl;
            this.buildHello = buildHello;
            this.startCoroutine = startCoroutine;
        }

        public static long NowMs() => (long)(Time.realtimeSinceStartupAsDouble * 1000.0);

        public void EnsurePumping()
        {
            if (pumping || stopped) return;
            pumping = startCoroutine(Pump());
        }

        public void Stop() => stopped = true;

        IEnumerator Pump()
        {
            try
            {
                while (!stopped)
                {
                    long now = NowMs();
                    if (session.TryBeginHello(now))
                    {
                        var hello = Hello();
                        while (hello.MoveNext()) yield return hello.Current;
                    }
                    else if (session.TryDequeueEvent(now, out var ev))
                    {
                        var send = PostEvent(ev);
                        while (send.MoveNext()) yield return send.Current;
                    }
                    else
                    {
                        yield return null;
                    }
                }
            }
            finally
            {
                pumping = false;
            }
        }

        IEnumerator Hello()
        {
            HelloRequest request;
            try
            {
                request = buildHello();
            }
            catch (Exception e)
            {
                log.Error("could not build hello: " + e);
                session.GoVanilla("hello could not be built");
                yield break;
            }

            var result = new HttpResult();
            var post = Post("/v1/hello", ProtocolJson.Serialize(request), HelloTimeoutMs, result);
            while (post.MoveNext()) yield return post.Current;

            if (result.Status == 200)
            {
                HelloResponse response = null;
                try
                {
                    response = ProtocolJson.Deserialize<HelloResponse>(result.Body);
                }
                catch (Exception e)
                {
                    log.Warn("unreadable hello response: " + e.Message);
                }
                session.HelloSucceeded(response);
                if (session.State == SessionState.Active)
                {
                    Appraisal.Guides = response.AppraisalGuides;
                    log.Info("bridge session " + session.SessionId + ", mode " + session.Mode
                        + (Appraisal.Guides == null ? "; no appraisal guides: the autopilot will not join the appraisal board" : "; appraisal guides loaded"));
                }
                else
                    LogVanilla();
            }
            else if (result.Status == 409)
            {
                session.HelloRejected("bridge refused protocol " + ProtocolVersion.Current + ": " + result.Body);
                LogVanilla();
            }
            else
            {
                session.HelloFailed(NowMs());
                log.Warn("hello failed (" + Describe(result) + "); breaker " + session.Breaker.State);
            }
        }

        IEnumerator PostEvent(EventRequest ev)
        {
            var result = new HttpResult();
            var post = Post("/v1/event", ProtocolJson.Serialize(ev), EventTimeoutMs, result);
            while (post.MoveNext()) yield return post.Current;

            if (result.Status == 204 || result.Status == 200) session.EventSucceeded();
            else if (result.Status == 404)
            {
                session.SessionUnknown();
                log.Info("bridge forgot session; saying hello again");
            }
            else
            {
                session.EventFailed(NowMs(), result.Transport);
                if (session.Breaker.State != Decisions.BreakerState.Closed)
                    log.Warn("event failed (" + Describe(result) + "); breaker " + session.Breaker.State + ", buffering " + session.QueuedEvents + " events");
            }
        }

        // P1+: ask the bridge, never longer than the session's deadline; `done` always runs, with defer on any failure.
        public void Decide(DecideRequest request, Action<DecisionOutcome> done)
        {
            var ticket = session.BeginDecision(request, NowMs());
            if (!ticket.Ask)
            {
                done(session.CompleteDecision(ticket, null, DecideFailure.None, NowMs()));
                return;
            }
            if (!startCoroutine(DecideRoutine(ticket, done)))
                done(session.CompleteDecision(ticket, null, DecideFailure.Transport, NowMs()));
        }

        IEnumerator DecideRoutine(DecisionTicket ticket, Action<DecisionOutcome> done)
        {
            DecisionOutcome outcome = null;
            var first = PostDecide(ticket, o => outcome = o);
            while (first.MoveNext()) yield return first.Current;

            if (outcome.RetryAfterHello)
            {
                log.Info("bridge forgot session during decide " + ticket.Request.DecisionId + "; saying hello again");
                EnsurePumping();
                while (!session.IsExpired(ticket, NowMs()) && (session.State == SessionState.NeedHello || session.State == SessionState.HelloInFlight))
                    yield return null;
                var retry = session.BeginRetry(ticket, NowMs());
                if (retry.Ask)
                {
                    var second = PostDecide(retry, o => outcome = o);
                    while (second.MoveNext()) yield return second.Current;
                }
                else
                {
                    outcome = session.CompleteDecision(retry, null, DecideFailure.None, NowMs());
                }
            }
            if (outcome.Choice.IsDefer) log.Info("decision " + ticket.Request.DecisionId + " deferred: " + outcome.Reason);
            done(outcome);
        }

        IEnumerator PostDecide(DecisionTicket ticket, Action<DecisionOutcome> result)
        {
            var http = new HttpResult();
            int budget = (int)Math.Max(1, ticket.StartedMs + ticket.EffectiveDeadlineMs - NowMs());
            var post = Post("/v1/decide", ProtocolJson.Serialize(ticket.Request), budget, http);
            while (post.MoveNext()) yield return post.Current;

            DecideResponse response = null;
            var failure = DecideFailure.None;
            if (http.TimedOut) failure = DecideFailure.Timeout;
            else if (http.Status == 404) failure = DecideFailure.UnknownSession;
            else if (http.Status != 200) failure = DecideFailure.Transport;
            else
            {
                try
                {
                    response = ProtocolJson.Deserialize<DecideResponse>(http.Body);
                }
                catch (Exception)
                {
                    failure = DecideFailure.BadResponse;
                }
            }
            result(session.CompleteDecision(ticket, response, failure, NowMs()));
        }

        public void SendOutcome(OutcomeRequest outcome)
        {
            if (session.State != SessionState.Active || session.Breaker.IsOpen(NowMs())) return;
            outcome.SessionId = session.SessionId;
            startCoroutine(OutcomeRoutine(outcome));
        }

        IEnumerator OutcomeRoutine(OutcomeRequest outcome)
        {
            var result = new HttpResult();
            var post = Post("/v1/outcome", ProtocolJson.Serialize(outcome), EventTimeoutMs, result);
            while (post.MoveNext()) yield return post.Current;
            if (result.Transport) session.Breaker.RecordFailure(NowMs());
        }

        // The overlay's day view. `done` always runs, with null on any failure.
        public void Summary(Action<SummaryResponse> done)
        {
            if (session.State != SessionState.Active || session.Breaker.IsOpen(NowMs()) || !startCoroutine(SummaryRoutine(session.SessionId, done)))
                done(null);
        }

        IEnumerator SummaryRoutine(string sessionId, Action<SummaryResponse> done)
        {
            var result = new HttpResult();
            var get = Send("GET", "/v1/summary?session_id=" + UnityWebRequest.EscapeURL(sessionId ?? ""), null, EventTimeoutMs, result);
            while (get.MoveNext()) yield return get.Current;
            SummaryResponse response = null;
            if (result.Status == 404)
            {
                session.SessionForgotten();
                EnsurePumping();
            }
            else if (result.Status == 200)
            {
                try
                {
                    response = ProtocolJson.Deserialize<SummaryResponse>(result.Body);
                }
                catch (Exception)
                {
                }
            }
            done(response);
        }

        IEnumerator Post(string path, string json, int timeoutMs, HttpResult result) => Send("POST", path, json, timeoutMs, result);

        IEnumerator Send(string method, string path, string json, int timeoutMs, HttpResult result)
        {
            string url = (baseUrl() ?? "").TrimEnd('/') + path;
            using (var req = new UnityWebRequest(url, method))
            {
                if (json != null)
                {
                    req.uploadHandler = new UploadHandlerRaw(Encoding.UTF8.GetBytes(json));
                    req.SetRequestHeader("Content-Type", "application/json");
                }
                req.downloadHandler = new DownloadHandlerBuffer();
                // Coarse backstop in whole seconds; the loop below enforces the real deadline.
                req.timeout = timeoutMs / 1000 + 2;
                long started = NowMs();
                var op = req.SendWebRequest();
                while (!op.isDone)
                {
                    if (NowMs() - started >= timeoutMs)
                    {
                        result.TimedOut = true;
                        req.Abort();
                        break;
                    }
                    yield return null;
                }
                if (!result.TimedOut)
                {
                    result.Status = req.responseCode;
                    result.Body = req.downloadHandler != null ? req.downloadHandler.text : null;
                }
                result.Error = req.error;
            }
        }

        void LogVanilla()
        {
            if (session.VanillaReason == lastVanillaLogged) return;
            lastVanillaLogged = session.VanillaReason;
            log.Warn("staying vanilla: " + session.VanillaReason);
        }

        static string Describe(HttpResult r) =>
            r.TimedOut ? "timeout" : r.Status == 0 ? (r.Error ?? "no connection") : "HTTP " + r.Status;
    }
}
