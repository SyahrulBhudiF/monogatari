(function () {
  var p = sessionStorage.pid;
  if (!p) {
    var a = new Uint8Array(16);
    crypto.getRandomValues(a);
    p = Array.from(a, function (x) { return x.toString(16).padStart(2, '0'); }).join('');
    sessionStorage.pid = p;
  }
  window.PID = p;
})();
