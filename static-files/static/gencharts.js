$(function () {
    $("#spinner").show();
    $("#spinnerTwo").show();
    $.getJSON(window.location.pathname + 'postsbyminute', function (data) {
        $("#spinner").hide();
        $("#postsByMinuteDiv").show();
        var currentime = new Date();
        var labels = Array.from(Array(1440).keys(), (x) => dateFns.addMinutes(Date(0,0,0), x));
        const ctx = document.getElementById("postsByMinute");
        new Chart(ctx, {
            type: 'line',
            data: {
                labels: labels,
                datasets: data
            },
            options: {
                elements: {
                    point: {
                        radius: 0
                    }
                },
                plugins: {
                    legend: {
                        display: false
                    },
                    tooltip: {
                        filter: function (tooltipItem) {
                            return tooltipItem.datasetIndex === 1;
                        },
                        callbacks: {
                            label: function(context) {
                                return context.parsed.y.toPrecision(3)+' posts per minute at '+dateFns.format(context.parsed.x, "hh:mm");
                            }
                        }
                    }

                },
                scales: {
                    x: {
                        type: 'time', 
                    },
                    y: {
                        beginAtZero: true
                    }
                }
            }
        });

    });
     $.getJSON(window.location.pathname + 'postsbydayall', function (data) {
        $("#spinnerTwo").hide();
        $("#postsByDayAllDiv").show();
        console.log(data);
        const ctx = document.getElementById("postsByDayAll");
        new Chart(ctx, {
            type: 'line',
            data: {
                datasets: data
            },
            options: {
                elements: {
                    point: {
                        radius: 0
                    }
                },
                plugins: {
                    legend: {
                        display: false
                    },
                    tooltip: {
                        filter: function (tooltipItem) {
                            return tooltipItem.datasetIndex === 1;
                        },
                        callbacks: {
                            label: function(context) {
                                return context.parsed.y.toString()+' posts on '+dateFns.format(context.parsed.x, 'LLL do, y');
                            }
                        }
                    }

                },
                scales: {
                    x: {
                        type: 'time', 
                    },
                    y: {
                        beginAtZero: true
                    }
                }
            }
        });
    });
});
