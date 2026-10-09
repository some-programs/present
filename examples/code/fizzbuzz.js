for (let i = 1; i <= 15; i++) {
  console.log((i % 3 ? '' : 'Fizz') + (i % 5 ? '' : 'Buzz') || i);
}
