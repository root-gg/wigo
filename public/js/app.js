angular.module('dialog', ['ui.bootstrap']).
    factory('$dialog', function ($rootScope, $modal) {

        var module = {};

        // alert dialog
        module.alert = function (data) {
            if (!data) return false;
            var options = {
                backdrop: true,
                backdropClick: true,
                templateUrl: 'static/partials/alertDialogPartial.html',
                controller: 'AlertDialogController',
                resolve: {
                    args: function () {
                        return {
                            data: angular.copy(data)
                        }
                    }
                }
            };
            module.openDialog(options);
        };

        // generic dialog
        $rootScope.dialogs = [];
        module.openDialog = function (options) {
            if (!options) return false;

            $.each($rootScope.dialogs, function (i, dialog) {
                dialog.close();
            });
            $rootScope.dialogs = [];

            $modal.open(options);
        };

        return module;
    });

var logLevels = [
    "DEBUG",
    "OK",
    "INFO",
    "ERROR",
    "WARNING",
    "CRITICAL",
    "EMERGENCY"
]

var logRowLevels = {
    "DEBUG"     : "",
    "OK"        : "success",
    "INFO"      : "info",
    "ERROR"     : "active",
    "WARNING"   : "warning",
    "CRITICAL"  : "danger",
    "EMERGENCY" : "dancer"
}

angular.module('wigo-filters', [])
    .filter('logLevelCssFilter', function() {
        return function(status,v) {
            return logRowLevels[logLevels[status - 1]];
        };
    })
    .filter('logLevelFilter', function() {
        return function(logs,minLevel){
            min = logLevels.indexOf(minLevel);
            return _.filter(logs, function(log){
                return log.Level >= min + 1;
            })
        };
    });

angular.module('wigo', ['ngRoute', 'dialog', 'restangular', 'wigo-filters'])
	.config(function($routeProvider) {
		$routeProvider
			.when('/',      { controller: HostsCtrl,    templateUrl:'partials/hosts.html',  reloadOnSearch: false })
			.when('/host',  { controller: HostCtrl,     templateUrl:'partials/host.html',   reloadOnSearch: false })
			.when('/logs',  { controller: LogsCtrl,     templateUrl:'partials/logs.html',   reloadOnSearch: false })
			.otherwise({ redirectTo: '/' });
    })
    .config(function(RestangularProvider) {
        RestangularProvider.setBaseUrl('/api');
    });

function HostsCtrl($scope, Restangular, $dialog, $route, $location) {
}

function HostCtrl($scope, $dialog, $route, $location) {
    $scope.hello = "world";
}

function LogsCtrl($scope, Restangular, $dialog, $route, $location) {

    var _logs = Restangular.all('logs');
    $scope.logLevels = logLevels;

    $scope.menu = {
        level : "OK"
    };

    $scope.load = function() {
        _logs = Restangular;
        if($scope.menu.host){
            _logs = _logs.one('hosts',  $scope.menu.host)
        } else if($scope.menu.group){
            _logs = _logs.one('groups', $scope.menu.group)
        }
        if($scope.menu.probe){
            _logs = _logs.one('probes', $scope.menu.probe)
        }
        _logs = _logs.all('logs');

        _logs.getList().then(function(logs) {
            $scope.logs = logs;
        });
    }

    $scope.init = function() {
        $scope.menu.group    = $location.search().group;
        $scope.menu.host     = $location.search().host;
        $scope.menu.probe    = $location.search().probe;
        $scope.load();
    }

    $scope.updateUrl = function(){
        $location.search({
            group : $scope.menu.group,
            host : $scope.menu.host,
            probe : $scope.menu.probe,
        });
    }

    $scope.remove_group = function() {
        delete $scope.menu.group;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.remove_host = function() {
        delete $scope.menu.host;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.remove_probe = function() {
        delete $scope.menu.probe;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.set_group = function(group) {
        $scope.menu.group = group;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.set_host = function(host) {
        $scope.menu.host = host;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.set_probe = function(probe) {
        $scope.menu.probe = probe;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.hello = "world";
    $scope.init();
}

function AlertDialogController($rootScope, $scope, $modalInstance, args) {
    $rootScope.dialogs.push($scope);

    $scope.title = 'Success !';
    if (args.data.status != 100) $scope.title = 'Oops !';

    $scope.data = args.data;

    $scope.close = function (result) {
        $rootScope.dialogs = _.without($rootScope.dialogs, $scope);
        $modalInstance.close(result);
        if(args.callback) {
            args.callback(result);
        }
    };
}
