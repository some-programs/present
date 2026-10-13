# Fence

##

Lines in a fenced code block are code:

```sh
# not a heading
.not a command
: not a note
// not a comment
\.kept as written
::: notes
:::
```

~~~~
```
## still code
~~~~

Lines that start with # but are not headings are text:

#hashtag
##x

---
<h1>Fence</h1>
<section>
<p>Lines in a fenced code block are code:</p>
<pre><code class="language-sh"># not a heading
.not a command
: not a note
// not a comment
\.kept as written
::: notes
:::
</code></pre>
<pre><code>```
## still code
</code></pre>
<p>Lines that start with # but are not headings are text:</p>
<p>#hashtag
##x</p>
</section>
