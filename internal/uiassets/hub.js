(function(){
  var t = localStorage.getItem('theme');
  if(!t){ t = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'; }
  document.documentElement.setAttribute('data-theme', t);
  var b = document.getElementById('theme-toggle');
  if(!b){ return; }
  function icon(){
    var m = document.documentElement.getAttribute('data-theme');
    b.textContent = m === 'dark' ? '☀' : '🌙';
  }
  icon();
  b.addEventListener('click', function(){
    var n = document.documentElement.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
    document.documentElement.setAttribute('data-theme', n);
    localStorage.setItem('theme', n);
    icon();
  });
})();
